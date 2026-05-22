/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package login

import (
	"context"
	"crypto/x509"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"

	publicv1 "github.com/osac-project/fulfillment-service/internal/api/osac/public/v1"
	"github.com/osac-project/fulfillment-service/internal/auth"
	"github.com/osac-project/fulfillment-service/internal/config"
	"github.com/osac-project/fulfillment-service/internal/exit"
	"github.com/osac-project/fulfillment-service/internal/logging"
	"github.com/osac-project/fulfillment-service/internal/network"
	"github.com/osac-project/fulfillment-service/internal/oauth"
	"github.com/osac-project/fulfillment-service/internal/terminal"
)

//go:embed templates
var templatesFS embed.FS

func Cmd() *cobra.Command {
	runner := &runnerContext{}
	result := &cobra.Command{
		Use:                   "login [FLAG...] ADDRESS",
		DisableFlagsInUseLine: true,
		Short:                 "Save connection and authentication details.",
		Long:                  longHelp,
		RunE:                  runner.run,
	}
	flags := result.Flags()
	flags.BoolVar(
		&runner.args.plaintext,
		"plaintext",
		false,
		plaintextFlagHelp,
	)
	flags.BoolVar(
		&runner.args.insecure,
		"insecure",
		false,
		insecureFlagHelp,
	)
	flags.StringArrayVar(
		&runner.args.caFiles,
		"ca-file",
		[]string{},
		caFilesFlagHelp,
	)
	flags.StringVar(
		&runner.args.address,
		"address",
		os.Getenv("OSAC_ADDRESS"),
		addressFlagHelp,
	)
	flags.BoolVar(
		&runner.args.private,
		"private",
		false,
		privateFlagHelp,
	)
	flags.StringVar(
		&runner.args.token,
		"token",
		os.Getenv("OSAC_TOKEN"),
		tokenFlagHelp,
	)
	flags.StringVar(
		&runner.args.tokenScript,
		"token-script",
		os.Getenv("OSAC_TOKEN_SCRIPT"),
		tokenScriptFlagHelp,
	)
	flags.StringVar(
		&runner.args.oauthIssuer,
		"oauth-issuer",
		"",
		oauthIssuerFlagHelp,
	)
	flags.StringVar(
		&runner.args.oauthFlow,
		"oauth-flow",
		string(oauth.DeviceFlow),
		oauthFlowFlagHelp,
	)
	flags.StringVar(
		&runner.args.oauthClientId,
		"oauth-client-id",
		"osac-cli",
		oauthClientIdFlagHelp,
	)
	flags.StringVar(
		&runner.args.oauthClientSecret,
		"oauth-client-secret",
		"",
		oauthClientSecretFlagHelp,
	)
	flags.StringSliceVar(
		&runner.args.oauthScopes,
		"oauth-scopes",
		[]string{},
		oauthScopesFlagHelp,
	)
	flags.StringVar(
		&runner.args.oauthRedirectUri,
		"oauth-redirect-uri",
		defaultRedirectUri,
		oauthRedirectUriFlagHelp,
	)
	flags.StringVar(
		&runner.args.oauthUser,
		"oauth-user",
		"",
		oauthUserFlagHelp,
	)
	flags.StringVar(
		&runner.args.oauthPassword,
		"oauth-password",
		"",
		oauthPasswordFlagHelp,
	)
	flags.MarkHidden("address")
	flags.MarkHidden("private")
	flags.MarkHidden("token")
	flags.MarkHidden("token-script")
	return result
}

type runnerContext struct {
	logger     *slog.Logger
	console    *terminal.Console
	flags      *pflag.FlagSet
	address    string
	plaintext  bool
	caPool     *x509.CertPool
	tokenStore auth.TokenStore
	args       struct {
		plaintext         bool
		insecure          bool
		caFiles           []string
		address           string
		private           bool
		token             string
		tokenScript       string
		oauthIssuer       string
		oauthFlow         string
		oauthClientId     string
		oauthClientSecret string
		oauthScopes       []string
		oauthRedirectUri  string
		oauthUser         string
		oauthPassword     string
	}
}

func (c *runnerContext) run(cmd *cobra.Command, args []string) error {
	var err error

	// Get the context:
	ctx := cmd.Context()

	// Get the logger, console and flags:
	c.logger = logging.LoggerFromContext(ctx)
	c.console = terminal.ConsoleFromContext(ctx)
	c.flags = cmd.Flags()

	// Load the templates for the console messages:
	err = c.console.AddTemplates(templatesFS, "templates")
	if err != nil {
		return fmt.Errorf("failed to load templates: %w", err)
	}

	// The address used to be specified with a command line flag, but now we also take it from the arguments:
	c.address = c.args.address
	if c.address == "" {
		if len(args) == 1 {
			c.address = args[0]
		} else {
			return fmt.Errorf("address is mandatory")
		}
	}

	// Parse the address:
	c.address, c.plaintext, err = c.parseAddress(c.address)
	if err != nil {
		return fmt.Errorf("failed to parse address: %w", err)
	}

	// Check if the plaintext flag has been explcitly set, and if it conflicts with the result of parsing the
	// address. If it does conflict, then explain the issue to the user.
	if c.flags.Changed("plaintext") && c.plaintext != c.args.plaintext {
		c.console.Render(ctx, "plaintext_conflict.txt", map[string]any{
			"Address":   c.address,
			"Plaintext": c.plaintext,
		})
		return exit.Error(1)
	}

	// Create the CA pool:
	c.caPool, err = network.NewCertPool().
		SetLogger(c.logger).
		AddSystemFiles(true).
		AddKubernetesFiles(true).
		AddFiles(c.args.caFiles...).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create CA pool: %w", err)
	}

	// Create an anonymous gRPC client that we will use to fetch the metadata:
	grpcConn, err := network.NewGrpcClient().
		SetLogger(c.logger).
		SetPlaintext(c.plaintext).
		SetInsecure(c.args.insecure).
		SetCaPool(c.caPool).
		SetAddress(c.address).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create anonymous gRPC connection: %w", err)
	}
	defer func() {
		if grpcConn != nil {
			err := grpcConn.Close()
			if err != nil {
				c.logger.ErrorContext(
					ctx,
					"Failed to close gRPC connection",
					slog.Any("error", err),
				)
			}
		}
	}()

	// Fetch the capabilities:
	capabilities, err := c.fetchCapabilities(ctx, grpcConn)
	if err != nil {
		return fmt.Errorf("failed to fetch capabilities: %w", err)
	}
	c.logger.DebugContext(
		ctx,
		"Fetched capabilities",
		slog.Any("capabilities", capabilities),
	)

	// Select the token issuer. The result may be no issuer, which means that no authentication will be used, it
	// will all be anonoymous.
	tokenIssuer, err := c.selectTokenIssuer(ctx, capabilities)
	if err != nil {
		return fmt.Errorf("failed to select token issuer: %w", err)
	}

	// Get the settings from the context, reset them to discard any previous configuration, and create a token store:
	cfg := config.SettingsFromContext(ctx)
	cfg.Reset()
	c.tokenStore = cfg.TokenStore()

	// Create the token source only if a token issuer has been selected.
	tokenSource, err := c.createTokenSource(ctx, tokenIssuer)
	if err != nil {
		return fmt.Errorf("failed to create token source: %w", err)
	}

	// If we got a token source, then try to obtain a token using it, as this will trigger the authentication flow
	// and verify that it works correctly.
	if tokenSource != nil {
		_, err = tokenSource.Token(ctx)
		if err != nil {
			return fmt.Errorf("failed to obtain token using token source: %w", err)
		}
	}

	// Save the basic details of the configuration:
	cfg.SetPlaintext(c.plaintext)
	cfg.SetInsecure(c.args.insecure)
	cfg.SetAddress(c.address)
	cfg.SetPrivate(c.args.private)

	// For CA files that are absolute we need to store only the path, but for those that are relative we need to
	// save the content because otherwise we will not be able to use them when the command is executed from a
	// different directory.
	for _, caFile := range c.args.caFiles {
		if filepath.IsAbs(caFile) {
			cfg.AddCaFile(config.CaFile{
				Name: caFile,
			})
		} else {
			caContent, err := os.ReadFile(caFile)
			if err != nil {
				return fmt.Errorf("failed to read CA file '%s': %w", caFile, err)
			}
			cfg.AddCaFile(config.CaFile{
				Name:    caFile,
				Content: string(caContent),
			})
		}
	}

	// Save the authenticatoin configuration. Note that the OAuth settings are only saved when they are actually
	// used, and they won't be actually used if the user selected to use a static token or a token script.
	if c.args.token != "" {
		cfg.SetAccessToken(c.args.token)
	} else if c.args.tokenScript != "" {
		cfg.SetTokenScript(c.args.tokenScript)
	} else if tokenIssuer != "" {
		cfg.SetOauthIssuer(tokenIssuer)
		cfg.SetOAuthFlow(oauth.Flow(c.args.oauthFlow))
		cfg.SetOAuthClientId(c.args.oauthClientId)
		cfg.SetOAuthClientSecret(c.args.oauthClientSecret)
		cfg.SetOAuthScopes(c.args.oauthScopes)
		cfg.SetOAuthRedirectUri(c.args.oauthRedirectUri)
		cfg.SetOAuthUser(c.args.oauthUser)
		cfg.SetOAuthPassword(c.args.oauthPassword)
	}

	// Replace the gRPC anonymous connection with the authenticated one:
	err = grpcConn.Close()
	if err != nil {
		return fmt.Errorf("failed to close anonymous gRPC connection: %w", err)
	}
	grpcConn, err = network.NewGrpcClient().
		SetLogger(c.logger).
		SetPlaintext(c.plaintext).
		SetInsecure(c.args.insecure).
		SetCaPool(c.caPool).
		SetAddress(c.address).
		SetTokenSource(tokenSource).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create authenticated gRPC connection: %w", err)
	}

	// Check if the configuration is working by invoking the health check method:
	healthClient := healthv1.NewHealthClient(grpcConn)
	healthResponse, err := healthClient.Check(ctx, &healthv1.HealthCheckRequest{})
	if err != nil {
		return err
	}
	if healthResponse.Status != healthv1.HealthCheckResponse_SERVING {
		return fmt.Errorf("server is not serving, status is '%s'", healthResponse.Status)
	}

	// Everything is working, so we can save the configuration:
	err = cfg.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	return nil
}

// parseAddress parses the address and returns the address and whether accoding to that address the connection should
// use plaintext, without TLS.
func (c *runnerContext) parseAddress(text string) (address string, plaintext bool, err error) {
	parser, err := network.NewAddressParser().
		SetLogger(c.logger).
		Build()
	if err != nil {
		err = fmt.Errorf("failed to create address parser: %w", err)
		return
	}
	address, plaintext, err = parser.Parse(text)
	return
}

func (c *runnerContext) fetchCapabilities(ctx context.Context,
	grpcConn *grpc.ClientConn) (result *publicv1.CapabilitiesGetResponse, err error) {
	capabilitiesClient := publicv1.NewCapabilitiesClient(grpcConn)
	result, err = capabilitiesClient.Get(ctx, publicv1.CapabilitiesGetRequest_builder{}.Build())
	return
}

func (c *runnerContext) selectTokenIssuer(ctx context.Context, capabilities *publicv1.CapabilitiesGetResponse) (result string, err error) {
	advertisedIssuers := capabilities.GetAuthn().GetTrustedTokenIssuers()
	if len(advertisedIssuers) > 0 {
		result = advertisedIssuers[0]
		if len(advertisedIssuers) > 1 {
			c.logger.WarnContext(
				ctx,
				"Server advertises multiple issuers, selecting the first one",
				slog.Any("advertised", advertisedIssuers),
				slog.Any("selected", result),
			)
		}
	} else {
		c.logger.WarnContext(
			ctx,
			"Server advertises no issuers",
			slog.Any("selected", result),
		)
	}
	return
}

// createTokenSource creates a token source from the configuration. The token source will be nil if no token, token
// script or token issuer has been specified.
func (c *runnerContext) createTokenSource(ctx context.Context, tokenIssuer string) (result auth.TokenSource, err error) {
	// Use a token if specified:
	if c.args.token != "" {
		result, err = auth.NewStaticTokenSource().
			SetLogger(c.logger).
			SetToken(&auth.Token{
				Access: c.args.token,
			}).
			Build()
		if err != nil {
			err = fmt.Errorf("failed to create static token source: %w", err)
		}
		return
	}

	// Use a token script if specified::
	if c.args.tokenScript != "" {
		result, err = auth.NewScriptTokenSource().
			SetLogger(c.logger).
			SetScript(c.args.tokenScript).
			SetStore(c.tokenStore).
			Build()
		if err != nil {
			err = fmt.Errorf("failed to create script token source: %w", err)
		}
		return
	}

	// If a token issuer has been selected, then use OAuth to create a token source:
	if tokenIssuer != "" {
		result, err = oauth.NewTokenSource().
			SetLogger(c.logger).
			SetStore(c.tokenStore).
			SetListener(&oauthFlowListener{
				runner: c,
			}).
			SetInsecure(c.args.insecure).
			SetCaPool(c.caPool).
			SetInteractive(true).
			SetIssuer(tokenIssuer).
			SetFlow(oauth.Flow(c.args.oauthFlow)).
			SetClientId(c.args.oauthClientId).
			SetClientSecret(c.args.oauthClientSecret).
			SetScopes(c.args.oauthScopes...).
			SetRedirectUri(c.args.oauthRedirectUri).
			SetUsername(c.args.oauthUser).
			SetPassword(c.args.oauthPassword).
			Build()
		if err != nil {
			err = fmt.Errorf("failed to create OAuth token source: %w", err)
		}
		return
	}

	// Finally, if there is no token, toke script or token issuer, return nil:
	result = nil
	return
}

type oauthFlowListener struct {
	runner *runnerContext
}

func (l *oauthFlowListener) Start(ctx context.Context, event oauth.FlowStartEvent) error {
	switch event.Flow {
	case oauth.CodeFlow:
		return l.startCodeFlow(ctx, event)
	case oauth.DeviceFlow:
		return l.startDeviceFlow(ctx, event)
	case oauth.CredentialsFlow, oauth.PasswordFlow:
		// These flows don't require user interaction, so there is nothing to do here.
		return nil
	default:
		return fmt.Errorf(
			"unsupported flow '%s', must be '%s', '%s', '%s' or '%s'",
			event.Flow, oauth.CodeFlow, oauth.DeviceFlow, oauth.CredentialsFlow, oauth.PasswordFlow,
		)
	}
}

func (l *oauthFlowListener) startCodeFlow(ctx context.Context, event oauth.FlowStartEvent) error {
	l.runner.console.Render(ctx, "start_code_flow.txt", map[string]any{
		"AuthorizationUri": event.AuthorizationUri,
	})
	return nil
}

func (l *oauthFlowListener) startDeviceFlow(ctx context.Context, event oauth.FlowStartEvent) error {
	// If the authorizatoin server has provided a complete URL, with the code included, then use it, otherwise use
	// the URL without the code:
	verficationUri := event.VerificationUriComplete
	if verficationUri == "" {
		verficationUri = event.VerificationUri
	}

	// Calculate the expiration time to show to the user::
	now := time.Now()
	expiresIn := humanize.RelTime(now, now.Add(event.ExpiresIn), "from now", "")
	l.runner.console.Render(ctx, "start_device_flow.txt", map[string]any{
		"VerificationUri": verficationUri,
		"UserCode":        event.UserCode,
		"ExpiresIn":       expiresIn,
	})
	return nil
}

func (l *oauthFlowListener) End(ctx context.Context, event oauth.FlowEndEvent) error {
	if event.Outcome {
		l.runner.console.Render(ctx, "auth_success.txt", nil)
	} else {
		l.runner.console.Render(ctx, "auth_failure.txt", nil)
	}
	return nil
}

// defaultRedirectUri is the default redirect URI used for the OAuth code flow. The value 'http://localhost:0' means
// binding to localhost on a randomly selected port.
const defaultRedirectUri = "http://localhost:0"

const longHelp = `
Save connection and authentication details.

The recommended way to specify the server address is to use a URL, including
the scheme, host name and optionally the port number. For example
{{ bt }}https://osac.example.com{{ bt }} will connect to the server at
{{ bt }}osac.example.com{{ bt }} using TLS and port 443.

With this format the {{ bt }}https{{ bt }} scheme indicates that TLS
should be used, and the {{ bt }}http{{ bt }} scheme indicates that plaintext
plaintext should be used.

Alternatively, you can specify the server address as a host name and port number, for
example {{ bt }}osac.example.com:8000{{ bt }} will connect to the server at
{{ bt }}osac.example.com{{ bt }} using TLS and port 8000.

Note that the connection always uses _gRPC_ on top of _HTTP/2_, regardless of the
format used to specify the server address.
`

const plaintextFlagHelp = `
_[BOOLEAN]_ - Enables or disables use of TLS for communications with the API
server. Disabling TLS is insecure and should only be used for testing purposes.
Note that TLS is also automatically enabled if the server address uses the
{{ bt }}http{{ bt }} scheme instead of the default
 {{ bt }}https{{ bt }}.
`

const insecureFlagHelp = `
_[BOOLEAN]_ - Enables or disables verification of TLS certificates and host
names of the OAuth and API servers. Disabling TLS verifications is insecure and
should only be used for testing purposes. If you need to trust a custom CA
certificate, consider using the {{ bt }}--ca-file{{ bt }} flag
instead.
`

const caFilesFlagHelp = `
_FILE|DIRECTORY_ - File or directory containing trusted CA certificates. When
a directory is specified, all the files in the directory with extensions
{{ bt }}.cer{{ bt }}, {{ bt }}.crt{{ bt }} and {{ bt }}.pem{{ bt }} are read
recursively.

The CA certificates trusted by the operating system are automatically loaded.

When the process is running inside a _Kuberneres_ pod the cluster and service
CA certificates are also automatically loaded; there is no need to specify them
explicitly.

It can be used multiple times to add multiple CA files or directories.
`

const addressFlagHelp = `
_URL|HOST:PORT_ - Server address.
`

const privateFlagHelp = `
_[BOOLEAN]_ - Enables use of the private API.
`

const tokenFlagHelp = `
_TOKEN_ - Authentication token.
`

const tokenScriptFlagHelp = `
_SCRIPT_ - Shell command that will be executed to obtain the token. For example, to
automatically get the token of the Kubernetes {{ bt }}client{{ bt }}
service account of the {{ bt }}example{{ bt }} namespace the
value could be like this:

{{ bt 3 }}shell
kubectl create token -n example client --duration 1h
{{ bt 3 }}

Note that is important to quote this shell command correctly, as it will be
passed to your shell for execution.
`

const oauthIssuerFlagHelp = `
_URL_ - OAuth issuer URL. This is optional. By default the issuer advertised by the
server is used.
`

const oauthFlowFlagHelp = `
_FLOW_ - OAuth flow to use. Must be one of 'code', 'device', 'credentials' or 'password'.
`

const oauthClientIdFlagHelp = `
_ID_ - OAuth client identifier. All authentication flows require a client identifier,
but for most of them the default {{ bt }}osac-cli{{ bt }} value is
appropriate. When using the {{ bt }}credentials{{ bt }} flow,
typically for service accounts, you must provide the client identifier of the
service account together with the {{ bt }}--client-secret{{ bt }}
flag.
`

const oauthClientSecretFlagHelp = `
_SECRET_ - OAuth client secret. When using the {{ bt }}credentials{{ bt }}
flow, typically for service accounts, you must provide the client secret of the
service account together with the together with the
{{ bt }}--client-id{{ bt }} flag.
`

const oauthScopesFlagHelp = `
_[SCOPE...]_ - Comma separated list of OAuth scopes to request.
`

const oauthRedirectUriFlagHelp = `
_URI_ - Redirect URI to use for the OAuth {{ bt }}code{{ bt }} flow. The
default value {{ bt }}http://localhost:0{{ bt }} means binding to
localhost on a randomly selected port.
`

const oauthUserFlagHelp = `
_USER_ - OAuth user name. This is required when using the
{{ bt }}password{{ bt }} flow and should be used together with the
 {{ bt }}--password{{ bt }} flag.
`

const oauthPasswordFlagHelp = `
_PASSWORD_ - OAuth password. This is required when using the {{ bt }}password{{ bt }}
flow and should be used together with the {{ bt }}--user{{ bt }} flag.
`

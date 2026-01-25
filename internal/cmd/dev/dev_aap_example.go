/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package dev

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/innabox/fulfillment-service/internal"
)

// NewAapExample creates a command that runs the AAP example.
func NewAapExample() *cobra.Command {
	runner := &aapExampleComandRunner{}
	command := &cobra.Command{
		Use:   "aap-example",
		Short: "Runs the AAP example",
		Args:  cobra.NoArgs,
		RunE:  runner.run,
	}
	return command
}

type aapExampleComandRunner struct {
	logger    *slog.Logger
	flags     *pflag.FlagSet
	host      string
	base      string
	token     string
	client    *http.Client
	csrfToken string
}

func (c *aapExampleComandRunner) run(cmd *cobra.Command, argv []string) error {
	var err error

	// Get the context:
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Get the dependencies from the context:
	c.logger = internal.LoggerFromContext(ctx)

	// Save the flags:
	c.flags = cmd.Flags()

	// Load the token and base URL:
	c.token = "..."
	c.host = "..."
	c.base = fmt.Sprintf("https://%s/api/controller/v2", c.host)
	c.csrfToken = uuid.NewString()

	// Create the HTTP client:
	var transport http.RoundTripper = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("failed to create cookie jar: %w", err)
	}
	c.client = &http.Client{
		Transport: transport,
		Jar:       jar,
	}

	// Create the websocket connection to receive notifications from the controller:
	wsOptions := &websocket.DialOptions{
		HTTPClient: c.client,
		HTTPHeader: http.Header{
			"Authorization": []string{fmt.Sprintf("Bearer %s", c.token)},
			"Cookie":        []string{fmt.Sprintf("csrftoken=%s", c.csrfToken)},
		},
	}
	wsUrl := fmt.Sprintf("wss://%s/api/controller/v2/websocket/", c.host)
	ws, _, err := websocket.Dial(ctx, wsUrl, wsOptions)
	if err != nil {
		return fmt.Errorf("failed to create websocket connection: %w", err)
	}
	defer ws.CloseNow()

	// Subscribe to notifications:
	subscriptionRequest := &SubscriptionRequest{
		XrfToken: c.csrfToken,
		Groups: map[string][]string{
			"jobs": {
				"status_changed",
			},
		},
	}
	err = wsjson.Write(ctx, ws, subscriptionRequest)
	if err != nil {
		return fmt.Errorf("failed to write subscription request: %w", err)
	}

	// Read the notifications:
	go func() {
		for {
			_, messageBytes, err := ws.Read(ctx)
			if err != nil {
				c.logger.ErrorContext(
					ctx,
					"Failed to read notification",
					slog.Any("error", err),
				)
				return
			}
			fmt.Printf("Message: %s\n", messageBytes)
			var notification NotificationMessage
			err = json.Unmarshal(messageBytes, &notification)
			if err != nil {
				c.logger.ErrorContext(
					ctx,
					"Failed to unmarshal notification",
					slog.Any("error", err),
				)
				return
			}
			if notification.GroupName == JobsGroupName {
				switch notification.Status {
				case "successful":
					c.jobsSuccessful(ctx, notification.UnifiedJobId)
				case "failed":
					c.jobsFailed(ctx, notification.UnifiedJobId)
				}
			}
		}
	}()

	// Find the job template:
	var jobTemplates List[JobTemplate]
	err = c.get(ctx, "job_templates/?name=osac-create-cluster", &jobTemplates)
	if err != nil {
		return fmt.Errorf("failed to get job templates: %w", err)
	}
	for _, jobTemplate := range jobTemplates.Results {
		fmt.Printf("%d: %s\n", jobTemplate.ID, jobTemplate.Name)
	}
	if jobTemplates.Count != 1 {
		return fmt.Errorf("expected exactly one job template, got %d", jobTemplates.Count)
	}
	jobTemplateId := jobTemplates.Results[0].ID

	// Lauch the job:
	var jobLaunchRequest JobLaunchRequest
	var jobLaunchResponse JobLaunchResponse
	err = c.post(ctx, fmt.Sprintf("job_templates/%d/launch", jobTemplateId), &jobLaunchRequest, &jobLaunchResponse)
	if err != nil {
		return fmt.Errorf("failed to launch job: %w", err)
	}
	fmt.Printf("Job launched: %d\n", jobLaunchResponse.ID)

	// Wait for the job to finish:
	time.Sleep(10 * time.Second)

	return nil
}

// get makes a GET request to the given URL and unmarshals the response into the given result.
func (c *aapExampleComandRunner) get(ctx context.Context, path string, result any) error {
	url := fmt.Sprintf("%s/%s", c.base, path)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("failed to do request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	fmt.Printf("%s\n", body)
	return json.Unmarshal(body, result)
}

// post makes a POST request to the given URL and request, and unmarshals the response.
func (c *aapExampleComandRunner) post(ctx context.Context, path string, request, response any) error {
	var err error
	url := fmt.Sprintf("%s/%s/", c.base, path)
	var requestReader io.Reader
	if request != nil {
		requestBytes, err := json.Marshal(request)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		requestReader = bytes.NewReader(requestBytes)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, url, requestReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	httpResponse, err := c.client.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("failed to do request: %w", err)
	}
	responseReader := httpResponse.Body
	defer responseReader.Close()
	responseBytes, err := io.ReadAll(responseReader)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	// fmt.Printf("%s\n", responseBytes)
	err = json.Unmarshal(responseBytes, response)
	if err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return nil
}

func (c *aapExampleComandRunner) jobsSuccessful(ctx context.Context, id int64) {
	fmt.Printf("Job %d successful\n", id)
	// Get the job:
	var job Job
	err := c.get(ctx, fmt.Sprintf("jobs/%d", id), &job)
	if err != nil {
		c.logger.ErrorContext(
			ctx,
			"Failed to get job",
			slog.Any("error", err),
		)
		return
	}
	fmt.Printf("Job artifacts:\n")
	for name, value := range job.Artifacts {
		fmt.Printf("- %s: %v\n", name, value)
	}
}

func (c *aapExampleComandRunner) jobsFailed(ctx context.Context, id int64) {
	fmt.Printf("Job %d failed\n", id)
}

// SubscriptionRequest represents a request to subscribe to notifications.
type SubscriptionRequest struct {
	Groups   map[string][]string `json:"groups"`
	XrfToken string              `json:"xrftoken"`
}

type NotificationMessage struct {
	GroupName          string `json:"group_name"`
	InstanceGroupName  string `json:"instance_group_name"`
	Status             string `json:"status"`
	Type               string `json:"type"`
	UnifiedJobId       int64  `json:"unified_job_id"`
	UnifiedJobTemplate int64  `json:"unified_job_template"`
}

const (
	JobsGroupName = "jobs"
)

// List is a generic list of objects.
type List[O any] struct {
	Count    int64   `json:"count"`
	Next     *string `json:"next"`
	Previous *string `json:"previous"`
	Results  []O     `json:"results"`
}

type JobLaunchRequest struct {
}

type JobLaunchResponse struct {
	ID int64 `json:"id"`
}

type Job struct {
	Id        int64          `json:"id"`
	Artifacts map[string]any `json:"artifacts"`
}

type JobTemplate struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	JobType     string `json:"job_type"`
	Inventory   int64  `json:"inventory"`
	Project     int64  `json:"project"`
	Playbook    string `json:"playbook"`
	SCMBranch   string `json:"scm_branch"`
	Forks       int64  `json:"forks"`
	Limit       string `json:"limit"`
}

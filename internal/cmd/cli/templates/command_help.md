# {{ .Name }}

{{ if .Long }}{{ evaluate .Long . }}{{ else }}{{ evaluate .Short . }}{{ end }}

## Usage

```shell
{{ .UseLine }}
```

{{ if .HasAvailableSubCommands }}
## Commands
{{ range .Commands }}
{{ if .IsAvailableCommand }}
* **{{ .Name }}** - {{ .Short }}.
{{ end }}
{{ end }}
{{ end }}

{{ if .HasAvailableLocalFlags }}
## Flags
{{ range flags .LocalFlags }}
{{ if and (not .Hidden) (ne .Name "help") }}
{{ execute "flag_help.md" . }}
{{ end }}
{{ end }}
{{ end }}

{{ if .HasAvailableInheritedFlags }}
## Global flags
{{ if not .Parent }}
{{ range flags .InheritedFlags }}
{{ execute "flag_help.md" . }}
{{ end }}
{{ else }}
For details about global flags, including logging and configuration, use `{{ binary }} -h`.
{{ end }}
{{ end }}
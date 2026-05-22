There are {{ .Total }} clusters matching name or identifier `{{ .Key }}`.

{{ if gt .Total (len .Ids) }}
These are the first {{ len .Ids }}:
{{ end }}

{{ range .Ids }}
- `{{ . -}}`
{{ end }}

To avoid this ambiguity, use the identifier. For example, to get the kubeconfig for cluster
`{{ index .Ids 0 }}`, use the following command:

```shell
{{ binary }} get kubeconfig {{ index .Ids 0 }}
```

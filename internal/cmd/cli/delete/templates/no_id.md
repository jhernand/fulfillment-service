You must specify the identifier or name of at least one object to delete. For example, to delete the
cluster with identifier `123`:

```shell
{{ binary }} delete cluster 123
```

Or to delete the cluster with name `my-cluster`:

```shell
{{ binary }} delete cluster my-cluster
```

You can also specify multiple identifiers or names. For example, to delete the clusters with
identifiers `123` and `456`:

```shell
{{ binary }} delete cluster 123 456
```

Use the `--help` option to get more details about the command.

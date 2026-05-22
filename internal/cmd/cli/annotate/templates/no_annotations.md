You must specify at least one annotation operation. For example, to set annotations on the cluster
with identifier `123`:

```shell
{{ binary }} annotate cluster 123 my-annotation=my-value your-annotation=your-value
```

To remove an annotation, use the annotation name followed by a `-` character:

```shell
{{ binary }} annotate cluster 123 my-annotation-
```

Use the `--help` option to get more details about the command.

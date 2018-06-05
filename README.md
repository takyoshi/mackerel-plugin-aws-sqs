mackerel-plugin-aws-sqs
=================================

```shell
mackerel-plugin-aws-sqs -queue-name <QueueName> [-region <aws_region>] [-access-key-id <id>] [-secret-access-key <key>] [-tempfile <tempfile>]
```

## Install

```
$ mkr plugin install takyoshi/mackerel-plugin-aws-sqs
```

## Config Example
```
[plugin.metrics.aws-sqs]
command = "/path/to/mackerel-plugin-aws-sqs -queue-name sample -region ap-northeast-1"
```

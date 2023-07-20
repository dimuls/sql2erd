# sql2erd

Generates ERD from SQL in SVG format. PostgreSQL only supported for now.

## How to use

```
$ sql2erd --help
Usage:
  sql2erd [flags]

Flags:
  -h, --help         help for sql2erd
      --in string     (default "-")
      --out string    (default "-")
```

## Example

Here is ERD of [example.sql](example.sql)

![Example ERD](example.svg?raw=true)

## License

sql2erd is licensed under [MIT](LICENSE.md).
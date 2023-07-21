# sql2erd

Generates ERD from SQL in SVG format. PostgreSQL only supported for now.

## How to use

```
$ sql2erd --help
Usage:
  sql2erd [flags]

Flags:
  -h, --help           help for sql2erd
  -i, --in string      path to input sql file, or "-" for stdin (default "-")
  -o, --out string     path to output svg file, or "-" for stdout (default "-")
  -t, --theme string   theme: "light" or "dark" (default "light")
```

## Example

Here is ERD of [example.sql](example.sql)

![Example ERD](example.svg?raw=true)

## License

sql2erd is licensed under [MIT](LICENSE.md).
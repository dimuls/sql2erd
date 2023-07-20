package main

import (
	"context"

	"github.com/dimuls/sql2erd/cmd/sql2erd/commands"
)

func main() {
	commands.Execute(context.Background())
}

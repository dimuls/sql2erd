package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dimuls/sql2erd"
)

var root = &cobra.Command{
	Use: "sql2erd",
	RunE: func(cmd *cobra.Command, args []string) error {
		var (
			ctx     = cmd.Context()
			inFile  = os.Stdin
			outFile = os.Stdout
			err     error
		)

		inFilePath, err := cmd.Flags().GetString("in")
		if err != nil {
			return fmt.Errorf("get in file: %w", err)
		}

		if inFilePath != "-" {
			inFile, err = os.Open(inFilePath)
			if err != nil {
				return err
			}

			defer inFile.Close()
		}

		outFilePath, err := cmd.Flags().GetString("out")
		if err != nil {
			return fmt.Errorf("get out file: %w", err)
		}

		if outFilePath != "-" {
			outFile, err = os.Create(outFilePath)
			if err != nil {
				return err
			}

			defer outFile.Close()
		}

		r := sql2erd.Renderer{
			In:  inFile,
			Out: outFile,
		}

		err = r.Render(ctx)
		if err != nil {
			return fmt.Errorf("render db scheme: %w", err)
		}

		return nil
	},
}

func init() {
	root.Flags().String("in", "-", "")
	root.Flags().String("out", "-", "")
}

func Execute(ctx context.Context) {
	err := root.ExecuteContext(ctx)
	if err != nil {
		os.Exit(1)
	}
}

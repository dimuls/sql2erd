package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dimuls/sql2erd"
)

var root = &cobra.Command{
	Use:          "sql2erd",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var (
			ctx     = cmd.Context()
			theme   sql2erd.Theme
			inFile  = os.Stdin
			outFile = os.Stdout
			err     error
		)

		themeStr, err := cmd.Flags().GetString("theme")
		if err != nil {
			return fmt.Errorf("get theme: %w", err)
		}

		switch themeStr {
		case "light":
			theme = sql2erd.LightTheme
		case "dark":
			theme = sql2erd.DarkTheme
		default:
			return fmt.Errorf("invalid theme: %s", themeStr)
		}

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
			Theme: theme,
			In:    inFile,
			Out:   outFile,
		}

		err = r.Render(ctx)
		if err != nil {
			return fmt.Errorf("render db scheme: %w", err)
		}

		return nil
	},
}

func init() {
	root.Flags().StringP("in", "i", "-", `path to input sql file, or "-" for stdin`)
	root.Flags().StringP("out", "o", "-", `path to output svg file, or "-" for stdout`)
	root.Flags().StringP("theme", "t", "light", `theme: "light" or "dark"`)
}

func Execute(ctx context.Context) {
	err := root.ExecuteContext(ctx)
	if err != nil {
		os.Exit(1)
	}
}

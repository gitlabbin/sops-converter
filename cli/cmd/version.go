package cmd

import (
	"fmt"
	"github.com/dhouti/sops-converter/pkg/version"
	"github.com/spf13/cobra"
	goruntime "runtime"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Long:  `Display version and build information about cli.`,
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

func printVersion() {
	fmt.Printf("Version: %s\n", version.AppVersion)
	fmt.Printf("Go Version: %s\n", goruntime.Version())
	fmt.Printf("Go OS/Arch: %s/%s\n", goruntime.GOOS, goruntime.GOARCH)
	fmt.Printf("Git Commit: %s\n", version.GitCommit)
	fmt.Printf("BuildDate: %s\n", version.BuildDate)
}

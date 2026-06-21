package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	runtimeCode  string
	runtimeLabel string
	runtimeID    string
)

var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Manage agent runtimes",
}

var runtimeRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a new runtime and get a runtime token",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtimeCode == "" {
			return fmt.Errorf("runtime code is required (--code)")
		}
		if runtimeLabel == "" {
			// fallback to code as label
			runtimeLabel = runtimeCode
		}

		mewClient, cfg, err := newMeworkClient()
		if err != nil {
			return err
		}

		patToken := cfg.Token
		if patToken == "" {
			return fmt.Errorf("not authenticated — run `mework login` first")
		}

		res, err := mewClient.CreateRuntime(patToken, runtimeCode, runtimeLabel)
		if err != nil {
			return err
		}

		fmt.Println("Runtime registered successfully!")
		fmt.Printf("ID:    %s\n", res.ID)
		fmt.Printf("Code:  %s\n", res.Code)
		fmt.Printf("Token: %s\n\n", res.Token)
		fmt.Println("IMPORTANT: Save the Token. It will NOT be shown again.")
		fmt.Printf("To configure this runtime for the daemon, run:\n")
		fmt.Printf("  mework config set rt_token %s\n", res.Token)

		return nil
	},
}

var runtimeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all runtimes registered under this account",
	RunE: func(cmd *cobra.Command, args []string) error {
		mewClient, cfg, err := newMeworkClient()
		if err != nil {
			return err
		}

		patToken := cfg.Token
		if patToken == "" {
			return fmt.Errorf("not authenticated — run `mework login` first")
		}

		runtimes, err := mewClient.ListRuntimes(patToken)
		if err != nil {
			return err
		}

		if len(runtimes) == 0 {
			fmt.Println("No runtimes registered.")
			return nil
		}

		fmt.Printf("%-36s %-15s %-15s %-10s %-25s\n", "ID", "Code", "Label", "Status", "Last Seen")
		for _, r := range runtimes {
			lastSeen := "never"
			if r.LastSeenAt != nil {
				lastSeen = r.LastSeenAt.Local().Format("2006-01-02 15:04:05")
			}
			fmt.Printf("%-36s %-15s %-15s %-10s %-25s\n", r.ID, r.Code, r.Label, r.Status, lastSeen)
		}
		return nil
	},
}

var runtimeRevokeCmd = &cobra.Command{
	Use:   "revoke",
	Short: "Revoke (delete) a registered runtime",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runtimeID == "" {
			return fmt.Errorf("runtime ID is required (--id)")
		}

		mewClient, cfg, err := newMeworkClient()
		if err != nil {
			return err
		}

		patToken := cfg.Token
		if patToken == "" {
			return fmt.Errorf("not authenticated — run `mework login` first")
		}

		err = mewClient.DeleteRuntime(patToken, runtimeID)
		if err != nil {
			return err
		}

		fmt.Printf("Runtime %s revoked successfully.\n", runtimeID)
		return nil
	},
}

func init() {
	runtimeRegisterCmd.Flags().StringVar(&runtimeCode, "code", "", "Unique code for the runtime (e.g. dev, macbook)")
	runtimeRegisterCmd.Flags().StringVar(&runtimeLabel, "label", "", "Friendly label for the runtime (optional)")
	_ = runtimeRegisterCmd.MarkFlagRequired("code")

	runtimeRevokeCmd.Flags().StringVar(&runtimeID, "id", "", "ID of the runtime to revoke")
	_ = runtimeRevokeCmd.MarkFlagRequired("id")

	runtimeCmd.AddCommand(runtimeRegisterCmd, runtimeListCmd, runtimeRevokeCmd)
}

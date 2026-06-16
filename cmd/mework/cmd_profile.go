package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"mework/internal/meworkclient"
)

var (
	profileNameField string
	profileBodyFile  string
	profileBackend   string
	profileHarness   string
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage AI config profiles on the server",
}

var profileCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new AI profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		if profileNameField == "" {
			return fmt.Errorf("profile name is required (--name)")
		}
		if profileBodyFile == "" {
			return fmt.Errorf("profile body file is required (--body)")
		}

		bodyBytes, err := os.ReadFile(profileBodyFile)
		if err != nil {
			return fmt.Errorf("failed to read body file: %w", err)
		}

		mewClient, cfg, err := newMeworkClient()
		if err != nil {
			return err
		}

		patToken := cfg.Token
		if patToken == "" {
			return fmt.Errorf("not authenticated — run `mework login` first")
		}

		req := meworkclient.CreateProfileRequest{
			Name:        profileNameField,
			Body:        string(bodyBytes),
			BackendHint: profileBackend,
			Harness:     profileHarness,
		}

		res, err := mewClient.CreateProfile(patToken, req)
		if err != nil {
			return err
		}

		fmt.Printf("Profile %q created successfully.\n", res.Name)
		return nil
	},
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all AI profiles registered on the server",
	RunE: func(cmd *cobra.Command, args []string) error {
		mewClient, cfg, err := newMeworkClient()
		if err != nil {
			return err
		}

		patToken := cfg.Token
		if patToken == "" {
			return fmt.Errorf("not authenticated — run `mework login` first")
		}

		profiles, err := mewClient.ListProfiles(patToken)
		if err != nil {
			return err
		}

		if len(profiles) == 0 {
			fmt.Println("No profiles registered.")
			return nil
		}

		fmt.Printf("%-20s %-12s %-12s %-25s\n", "Name", "Backend", "Harness", "Updated At")
		for _, p := range profiles {
			fmt.Printf("%-20s %-12s %-12s %-25s\n", p.Name, p.BackendHint, p.Harness, p.UpdatedAt)
		}
		return nil
	},
}

var profileUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update an existing AI profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		if profileNameField == "" {
			return fmt.Errorf("profile name is required (--name)")
		}
		if profileBodyFile == "" {
			return fmt.Errorf("profile body file is required (--body)")
		}

		bodyBytes, err := os.ReadFile(profileBodyFile)
		if err != nil {
			return fmt.Errorf("failed to read body file: %w", err)
		}

		mewClient, cfg, err := newMeworkClient()
		if err != nil {
			return err
		}

		patToken := cfg.Token
		if patToken == "" {
			return fmt.Errorf("not authenticated — run `mework login` first")
		}

		req := meworkclient.UpdateProfileRequest{
			Body:        string(bodyBytes),
			BackendHint: profileBackend,
			Harness:     profileHarness,
		}

		res, err := mewClient.UpdateProfile(patToken, profileNameField, req)
		if err != nil {
			return err
		}

		fmt.Printf("Profile %q updated successfully.\n", res.Name)
		return nil
	},
}

var profileDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete an AI profile from the server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if profileNameField == "" {
			return fmt.Errorf("profile name is required (--name)")
		}

		mewClient, cfg, err := newMeworkClient()
		if err != nil {
			return err
		}

		patToken := cfg.Token
		if patToken == "" {
			return fmt.Errorf("not authenticated — run `mework login` first")
		}

		err = mewClient.DeleteProfile(patToken, profileNameField)
		if err != nil {
			return err
		}

		fmt.Printf("Profile %q deleted successfully.\n", profileNameField)
		return nil
	},
}

func init() {
	profileCreateCmd.Flags().StringVar(&profileNameField, "name", "", "Name of the profile (e.g. dev, prod)")
	profileCreateCmd.Flags().StringVar(&profileBodyFile, "body", "", "Path to the file containing system prompt body")
	profileCreateCmd.Flags().StringVar(&profileBackend, "backend", "claude", "Backend hint (e.g. claude, codex, opencode)")
	profileCreateCmd.Flags().StringVar(&profileHarness, "harness", "claude-code", "Harness type (e.g. claude-code, custom)")
	_ = profileCreateCmd.MarkFlagRequired("name")
	_ = profileCreateCmd.MarkFlagRequired("body")

	profileUpdateCmd.Flags().StringVar(&profileNameField, "name", "", "Name of the profile (e.g. dev, prod)")
	profileUpdateCmd.Flags().StringVar(&profileBodyFile, "body", "", "Path to the file containing system prompt body")
	profileUpdateCmd.Flags().StringVar(&profileBackend, "backend", "claude", "Backend hint (e.g. claude, codex, opencode)")
	profileUpdateCmd.Flags().StringVar(&profileHarness, "harness", "claude-code", "Harness type (e.g. claude-code, custom)")
	_ = profileUpdateCmd.MarkFlagRequired("name")
	_ = profileUpdateCmd.MarkFlagRequired("body")

	profileDeleteCmd.Flags().StringVar(&profileNameField, "name", "", "Name of the profile to delete")
	_ = profileDeleteCmd.MarkFlagRequired("name")

	profileCmd.AddCommand(profileCreateCmd, profileListCmd, profileUpdateCmd, profileDeleteCmd)
}

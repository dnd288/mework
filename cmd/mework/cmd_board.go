package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "Manage workspaces",
	GroupID: groupCore,
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List accessible workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newRESTClient(cmd)
		if err != nil {
			return err
		}
		workspaces, err := client.ListWorkspaces()
		if err != nil {
			return err
		}
		if jsonFlag {
			return printJSON(workspaces)
		}
		w := newTable()
		row(w, "ID", "NAME", "ROLE")
		for _, ws := range workspaces {
			row(w, ws.ID, ws.Name, ws.Role)
		}
		return w.Flush()
	},
}

var boardCmd = &cobra.Command{
	Use:     "board",
	Short:   "Manage boards",
	GroupID: groupCore,
}

var boardListCmd = &cobra.Command{
	Use:   "list",
	Short: "List boards in the workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, cfg, err := newRESTClient(cmd)
		if err != nil {
			return err
		}
		ws, err := requireWorkspaceID(cmd, cfg)
		if err != nil {
			return err
		}
		boards, err := client.ListWorkspaceBoards(ws)
		if err != nil {
			return err
		}
		if jsonFlag {
			return printJSON(boards)
		}
		w := newTable()
		row(w, "ID", "CODE", "NAME")
		for _, b := range boards {
			row(w, b.ID, b.Code, b.Name)
		}
		return w.Flush()
	},
}

var boardGetCmd = &cobra.Command{
	Use:   "get <board-id>",
	Short: "Get a board with columns and tickets",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newRESTClient(cmd)
		if err != nil {
			return err
		}
		board, err := client.GetBoard(args[0])
		if err != nil {
			return err
		}
		if jsonFlag {
			return printJSON(board)
		}
		fmt.Printf("%s (%s)\n", board.Name, board.Code)
		w := newTable()
		row(w, "COLUMN", "POSITION", "TICKETS")
		for _, c := range board.Columns {
			row(w, c.Name, strconv.Itoa(c.Position), strconv.Itoa(c.TicketCount))
		}
		return w.Flush()
	},
}

func init() {
	workspaceListCmd.Flags().BoolVar(&jsonFlag, "json", false, "output as JSON")
	boardListCmd.Flags().BoolVar(&jsonFlag, "json", false, "output as JSON")
	boardGetCmd.Flags().BoolVar(&jsonFlag, "json", false, "output as JSON")
	workspaceCmd.AddCommand(workspaceListCmd)
	boardCmd.AddCommand(boardListCmd, boardGetCmd)
}

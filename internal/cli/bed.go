package cli

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/charris/hugel/internal/config"
)

func runBed(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(`hugel bed — the projects the garden knows

usage:
  hugel bed kin <name> <other>...   record that these names are one project
  hugel bed list                    show what the garden knows about beds

A project renamed across rewrites leaves knowledge filed under every name it
ever had. Without kinship, soil drawn in the new bed penalises the old bed's
entries as another project's business -- which means a project's oldest and
most settled decisions rank lowest exactly because they are old.
`)
		return nil
	}
	switch args[0] {
	case "kin":
		return runBedKin(args[1:])
	case "list":
		return runBedList(args[1:])
	default:
		return fmt.Errorf("unknown bed command %q", args[0])
	}
}

func runBedKin(args []string) error {
	fs := flag.NewFlagSet("bed kin", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("need a bed and at least one other name")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.AddKin(fs.Arg(0), fs.Args()[1:]...)
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("%s is also known as %s\n", fs.Arg(0), strings.Join(cfg.KinOf(fs.Arg(0)), ", "))
	return nil
}

func runBedList(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Kin) == 0 {
		fmt.Println("no bed kinship recorded")
		return nil
	}
	names := make([]string, 0, len(cfg.Kin))
	for k := range cfg.Kin {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Printf("%-16s %s\n", n, strings.Join(cfg.Kin[n], ", "))
	}
	return nil
}

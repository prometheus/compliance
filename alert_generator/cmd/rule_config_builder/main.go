package main

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-kit/log/level"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/compliance/alert_generator/cases"
	"github.com/prometheus/prometheus/model/rulefmt"
	yaml "gopkg.in/yaml.v3"
)

func main() {
	rulesFilePath := flag.String("rules-file-path", "./rules.yaml", "File path to write the rules file.")
	flag.Parse()
	log := promlog.New(&promlog.Config{})

	rgs := rulefmt.RuleGroups{
		Groups: make([]rulefmt.RuleGroup, 0, len(cases.AllCases())),
	}
	for _, c := range cases.AllCases() {
		rg, err := c.RuleGroup()
		if err != nil {
			title, _ := c.Describe()
			level.Error(log).Log("msg", "Failed to get rule group for a test case", "title", title, "err", err)
			os.Exit(1)
		}
		rgs.Groups = append(rgs.Groups, rg)
	}

	b, err := yaml.Marshal(rgs)
	if err != nil {
		level.Error(log).Log("msg", "Failed to marshal the rules", "err", err)
		os.Exit(1)
	}

	path, err := filepath.Abs(*rulesFilePath)
	if err != nil {
		level.Error(log).Log("msg", "Failed to get absolute path for the rules file", "path_from_flag", *rulesFilePath, "err", err)
		os.Exit(1)
	}

	err = os.WriteFile(path, b, fs.ModePerm)
	if err != nil {
		level.Error(log).Log("msg", "Failed to write the rules file", "err", err)
		os.Exit(1)
	}

	level.Info(log).Log("msg", "Rules file successfully generated", "path", path)
}

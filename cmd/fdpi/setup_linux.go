package main

import (
	_ "embed"
	"strings"
)

//go:embed install-guide.txt
var setupGuide string

func installationGuide() string {
	openProject := "Откройте терминал в распакованной папке FDPI с файлами Makefile и go.mod."
	if dir := findSource(); dir != "" {
		openProject = "cd -- '" + strings.ReplaceAll(dir, "'", "'\"'\"'") + "'"
	}
	return strings.ReplaceAll(setupGuide, "{{OPEN_PROJECT}}", openProject)
}

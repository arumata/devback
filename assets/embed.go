package assets

import "embed"

// GitTemplatesDir is the embedded directory with git hook templates.
const GitTemplatesDir = "git-templates"

// GitTemplatesFS embeds git hook templates.
//
//go:embed git-templates/*
var GitTemplatesFS embed.FS

// RepoTemplatesDir is the embedded directory with repository templates.
const RepoTemplatesDir = "repo-templates"

// RepoTemplatesFS embeds repository templates (e.g. .devbackignore).
//
//go:embed repo-templates/*
var RepoTemplatesFS embed.FS

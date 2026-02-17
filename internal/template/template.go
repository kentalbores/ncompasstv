// Package template defines screen layout templates for multi-zone
// digital signage. Each template divides the screen into rectangular
// zones, each with its own independent content playlist.
package template

import (
	"encoding/json"
	"fmt"
	"os"
)

// Zone represents a rectangular region of the screen.
// Coordinates are percentages (0-100) of total screen area.
type Zone struct {
	ID          string `json:"id"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	PlaylistDir string `json:"playlist_dir"`
	Zindex      int    `json:"zindex"`
}

// Template is a named screen layout with one or more zones.
type Template struct {
	Name  string `json:"name"`
	Zones []Zone `json:"zones"`
}

// LoadFromFile reads a template definition from a JSON file.
func LoadFromFile(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}

	var t Template
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	if err := t.Validate(); err != nil {
		return nil, err
	}

	return &t, nil
}

// Validate checks that the template has at least one zone and
// that all zones have valid dimensions.
func (t *Template) Validate() error {
	if len(t.Zones) == 0 {
		return fmt.Errorf("template %q has no zones", t.Name)
	}

	ids := make(map[string]bool)
	for _, z := range t.Zones {
		if z.ID == "" {
			return fmt.Errorf("zone missing id")
		}
		if ids[z.ID] {
			return fmt.Errorf("duplicate zone id: %s", z.ID)
		}
		ids[z.ID] = true

		if z.Width <= 0 || z.Height <= 0 {
			return fmt.Errorf("zone %q has invalid dimensions: %dx%d", z.ID, z.Width, z.Height)
		}
		if z.X < 0 || z.Y < 0 || z.X+z.Width > 100 || z.Y+z.Height > 100 {
			return fmt.Errorf("zone %q exceeds screen bounds", z.ID)
		}
	}

	return nil
}

// Fullscreen returns a single-zone template that fills the entire screen.
// This is the default template used for testing and simple deployments.
func Fullscreen(playlistDir string) *Template {
	return &Template{
		Name: "fullscreen",
		Zones: []Zone{
			{
				ID:          "main",
				X:           0,
				Y:           0,
				Width:       100,
				Height:      100,
				PlaylistDir: playlistDir,
				Zindex:      0,
			},
		},
	}
}

// MainWithFooter returns a two-zone template: a large main area
// and a horizontal footer strip at the bottom.
func MainWithFooter(mainDir, footerDir string) *Template {
	return &Template{
		Name: "main-with-footer",
		Zones: []Zone{
			{
				ID:          "main",
				X:           0,
				Y:           0,
				Width:       100,
				Height:      85,
				PlaylistDir: mainDir,
				Zindex:      0,
			},
			{
				ID:          "footer",
				X:           0,
				Y:           85,
				Width:       100,
				Height:      15,
				PlaylistDir: footerDir,
				Zindex:      1,
			},
		},
	}
}

// MainWithSidebar returns a two-zone template: a main content area
// and a vertical sidebar on the right.
func MainWithSidebar(mainDir, sideDir string) *Template {
	return &Template{
		Name: "main-with-sidebar",
		Zones: []Zone{
			{
				ID:          "main",
				X:           0,
				Y:           0,
				Width:       75,
				Height:      100,
				PlaylistDir: mainDir,
				Zindex:      0,
			},
			{
				ID:          "sidebar",
				X:           75,
				Y:           0,
				Width:       25,
				Height:      100,
				PlaylistDir: sideDir,
				Zindex:      1,
			},
		},
	}
}

// LShape returns a three-zone "L" layout: main content, footer, and sidebar.
func LShape(mainDir, footerDir, sideDir string) *Template {
	return &Template{
		Name: "l-shape",
		Zones: []Zone{
			{
				ID:          "main",
				X:           0,
				Y:           0,
				Width:       75,
				Height:      85,
				PlaylistDir: mainDir,
				Zindex:      0,
			},
			{
				ID:          "sidebar",
				X:           75,
				Y:           0,
				Width:       25,
				Height:      100,
				PlaylistDir: sideDir,
				Zindex:      1,
			},
			{
				ID:          "footer",
				X:           0,
				Y:           85,
				Width:       75,
				Height:      15,
				PlaylistDir: footerDir,
				Zindex:      2,
			},
		},
	}
}

package main

import (
	"log"

	"tree-sit/test/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	p := tea.NewProgram(ui.NewModel())

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

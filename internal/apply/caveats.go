package apply

// Caveat is the note a recipe asks ralph to show after one of its packages
// was built or installed.
type Caveat struct {
	Recipe string
	Text   string
}

// collectCaveat queues the caveat of the named recipe, if it has one.
func (s *sequence) collectCaveat(recipeName string) {
	for _, r := range s.cfg.LoadedRecipes {
		if r.Name == recipeName && r.Caveats != "" {
			s.caveats = append(s.caveats, Caveat{Recipe: recipeName, Text: r.Caveats})
			return
		}
	}
}

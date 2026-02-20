package gollem

// Toolset groups tools for modular management.
type Toolset struct {
	Name  string
	Tools []Tool
}

// NewToolset creates a named toolset.
func NewToolset(name string, tools ...Tool) *Toolset {
	return &Toolset{
		Name:  name,
		Tools: tools,
	}
}

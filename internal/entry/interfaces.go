package entry

// provider contributes entries to the registry. Adding subject matter to
// describe means adding a provider, never a command or a flag.
type provider interface {
	// Name identifies the provider on an entry.
	Name() string
	// Entries returns everything the provider can explain.
	Entries() ([]Entry, error)
}

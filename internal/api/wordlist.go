package api

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// adjectives and nouns are a small embedded wordlist used to generate
// friendly, Docker-container-style "adjective-noun" worktree name
// suggestions, so no external dependency is needed for this.
var adjectives = []string{
	"amber", "brisk", "calm", "dusty", "eager", "faint", "gentle", "hollow",
	"idle", "jolly", "keen", "lively", "misty", "nimble", "opal", "plucky",
	"quiet", "rusty", "sunny", "tidy", "urban", "vivid", "witty", "young",
	"zesty", "bold", "crisp", "dapper", "earnest", "fuzzy",
}

var nouns = []string{
	"otter", "falcon", "harbor", "meadow", "ridge", "canyon", "willow",
	"badger", "comet", "delta", "ember", "fjord", "grove", "heron", "island",
	"juniper", "kestrel", "lagoon", "marsh", "nebula", "orchard", "pebble",
	"quartz", "raven", "summit", "thicket", "umber", "vale", "wren", "yarrow",
}

// randomName returns a randomly generated "adjective-noun" name, e.g.
// "brisk-otter", for prefilling the new-worktree dialog.
func randomName() (string, error) {
	adj, err := randomChoice(adjectives)
	if err != nil {
		return "", err
	}
	noun, err := randomChoice(nouns)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", adj, noun), nil
}

func randomChoice(words []string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(words))))
	if err != nil {
		return "", fmt.Errorf("generate random index: %w", err)
	}
	return words[n.Int64()], nil
}

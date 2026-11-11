package main

import (
	"fmt"

	"github.com/mikebarb/labriideas-publisher/pkg/metadata"
)

func main() {
	inputs := []string{
		"How Does Jesus Relate to Unbelief =?utf-8?Q?=3F?=",
		"Do We Have an Answer to Humanism =?utf-8?Q?=3F?=",
		"A Culture of Commodification: How Much is for =?utf-8?Q?Sale=3F?=",
		"Plain title with no encoding",
	}

	for _, in := range inputs {
		out := metadata.Decode(in)
		fmt.Printf("INPUT:  %q\nOUTPUT: %q\n\n", in, out)
	}
}

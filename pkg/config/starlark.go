package config

import (
	"fmt"
	"log"
	"os"

	"github.com/babelsuite/babelsuite/pkg/engine"
	"go.starlark.net/starlark"
)

func ParseStarlark(file string) (*engine.Pipeline, error) {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return &engine.Pipeline{
			Name: "default",
			Tasks: []engine.Task{
				{Name: "example-sim", Image: "alpine", Command: []string{"echo", "BabelSuite orchestrator running!"}},
			},
		}, nil
	}

	thread := &starlark.Thread{
		Name:  "babelsuite main",
		Print: func(_ *starlark.Thread, msg string) { log.Println("STARLARK:", msg) },
	}

	globals, err := starlark.ExecFile(thread, file, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("starlark eval error: %v", err)
	}

	pipeline := &engine.Pipeline{Name: "starlark-run"}

	val := globals["pipeline"]
	if list, ok := val.(*starlark.List); ok {
		iter := list.Iterate()
		defer iter.Done()
		var pVal starlark.Value
		for iter.Next(&pVal) {
			if dict, ok := pVal.(*starlark.Dict); ok {
				name, _, _ := dict.Get(starlark.String("name"))
				image, _, _ := dict.Get(starlark.String("image"))

				pipeline.Tasks = append(pipeline.Tasks, engine.Task{
					Name:  name.(starlark.String).String(),
					Image: image.(starlark.String).String(),
				})
			}
		}
	}

	return pipeline, nil
}

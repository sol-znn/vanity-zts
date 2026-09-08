package main

import "encoding/json"

func jsonMarshalIndent(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

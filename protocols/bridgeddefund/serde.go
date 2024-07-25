package bridgeddefund

import (
	"encoding/json"
)

// TODO: Implement marshal json
func (o Objective) MarshalJSON() ([]byte, error) {
	return json.Marshal(o)
}

// TODO: Implement unmarshal json
func (o *Objective) UnmarshalJSON(data []byte) error {
	return nil
}

package helpers

import (
	"encoding/json"
	"fmt"

	"mesh-cu/internal/types"
)

// Encode упаковывает заголовок и полезную нагрузку в JSON.
func Encode(header types.Header, payload interface{}) ([]byte, error) {
	temp := struct {
		types.Header
		Payload interface{} `json:"payload"`
	}{
		Header:  header,
		Payload: payload,
	}
	return json.Marshal(temp)
}

// Decode разбирает JSON-сообщение на заголовок и payload-мапу.
func Decode(data []byte) (types.Header, map[string]interface{}, error) {
	var temp struct {
		types.Header
		Payload json.RawMessage `json:"payload"`
	}
	err := json.Unmarshal(data, &temp)
	if err != nil {
		return types.Header{}, nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	var payloadMap map[string]interface{}
	if len(temp.Payload) > 0 && string(temp.Payload) != "null" {
		err = json.Unmarshal(temp.Payload, &payloadMap)
		if err != nil {
			return temp.Header, nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
	}

	return temp.Header, payloadMap, nil
}

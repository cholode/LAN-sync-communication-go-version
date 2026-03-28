package api

import (
	"encoding/json"
	"os"
	"time"
)

// #region agent log
const agentDebugLogPath = "debug-3c90a5.log"

func agentDebugLog(hypothesisId, location, message string, data map[string]any) {
	f, err := os.OpenFile(agentDebugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	payload := map[string]any{
		"sessionId":    "3c90a5",
		"hypothesisId": hypothesisId,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	b, _ := json.Marshal(payload)
	_, _ = f.Write(append(b, '\n'))
}

// #endregion

package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentParams_UseSmallModel(t *testing.T) {
	t.Parallel()

	t.Run("default is false", func(t *testing.T) {
		t.Parallel()

		var params AgentParams
		err := json.Unmarshal([]byte(`{"prompt": "test"}`), &params)
		require.NoError(t, err)
		assert.Equal(t, "test", params.Prompt)
		assert.False(t, params.UseSmallModel, "UseSmallModel should default to false")
	})

	t.Run("can set to true", func(t *testing.T) {
		t.Parallel()

		var params AgentParams
		err := json.Unmarshal([]byte(`{"prompt": "test", "use_small_model": true}`), &params)
		require.NoError(t, err)
		assert.Equal(t, "test", params.Prompt)
		assert.True(t, params.UseSmallModel, "UseSmallModel should be true when set")
	})

	t.Run("can set to false explicitly", func(t *testing.T) {
		t.Parallel()

		var params AgentParams
		err := json.Unmarshal([]byte(`{"prompt": "test", "use_small_model": false}`), &params)
		require.NoError(t, err)
		assert.Equal(t, "test", params.Prompt)
		assert.False(t, params.UseSmallModel, "UseSmallModel should be false when set explicitly")
	})

	t.Run("serializes correctly with omitempty", func(t *testing.T) {
		t.Parallel()

		// When UseSmallModel is false (zero value), it should be omitted.
		params := AgentParams{Prompt: "test", UseSmallModel: false}
		data, err := json.Marshal(params)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "use_small_model", "use_small_model should be omitted when false")

		// When UseSmallModel is true, it should be included.
		params = AgentParams{Prompt: "test", UseSmallModel: true}
		data, err = json.Marshal(params)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"use_small_model":true`, "use_small_model should be present when true")
	})
}

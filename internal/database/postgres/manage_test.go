package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"external-db-operator/internal/database"
)

func TestProvider_GetDSN(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		input    string
		expected database.ConnectionInfo
	}{
		{
			name:     "success",
			input:    "postgres://postgres:postgres@localhost:5432/postgres",
			expected: database.ConnectionInfo{Host: "localhost", Port: 5432},
		},
		{
			name:     "success with password",
			input:    "postgres://foo:bar@1.2.3.4:3040/postgres",
			expected: database.ConnectionInfo{Host: "1.2.3.4", Port: 3040},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			provider := Provider{
				dsn: testCase.input,
			}

			actual := provider.GetConnectionInfo()
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

package mysql

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
			input:    "username:password@protocol(mysql:3306)/dbname?param=value",
			expected: database.ConnectionInfo{Host: "mysql", Port: 3306},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			provider := Provider{
				dsn: testCase.input,
			}

			actual, getConnectionInfoError := provider.GetConnectionInfo()
			assert.NoError(t, getConnectionInfoError)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

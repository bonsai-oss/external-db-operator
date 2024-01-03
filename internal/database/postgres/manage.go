package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"external-db-operator/internal/database"
	"external-db-operator/internal/helper"
)

func init() {
	database.RegisterProvider("postgres", Provide)
}

func Provide() database.Provider {
	return &Provider{}
}

type Provider struct {
	dsn          string
	dbConnection *pgx.Conn
}

var _ database.Provider = &Provider{}

func (p *Provider) GetConnectionInfo() (database.ConnectionInfo, error) {
	conn, dsnParseError := pgx.ParseConfig(p.dsn)
	if dsnParseError != nil {
		return database.ConnectionInfo{}, dsnParseError
	}
	return database.ConnectionInfo{
		Host: conn.Host,
		Port: conn.Port,
	}, nil
}

func (p *Provider) Apply(options database.CreateOptions) error {
	slog.Info("creating database", slog.String("name", options.Name))
	_, createDatabaseError := p.dbConnection.Exec(context.Background(), fmt.Sprintf("CREATE DATABASE %q", options.Name))
	if createDatabaseError != nil && !helper.IsAlreadyExistsError(createDatabaseError) {
		return createDatabaseError
	}

	var userExists bool
	if checkUserError := p.dbConnection.QueryRow(context.Background(), "SELECT EXISTS (SELECT FROM pg_roles WHERE rolname = $1)", options.Name).Scan(&userExists); checkUserError != nil {
		return checkUserError
	}

	if userExists {
		slog.Info("alter user", slog.String("name", options.Name))
		_, updateUserError := p.dbConnection.Exec(context.Background(), fmt.Sprintf("ALTER USER %s WITH PASSWORD '%s'", options.Name, options.Password))
		if updateUserError != nil {
			return updateUserError
		}
	} else {
		slog.Info("create user", slog.String("name", options.Name))
		_, createUserError := p.dbConnection.Exec(context.Background(), fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s'", options.Name, options.Password))
		if createUserError != nil {
			return createUserError
		}
	}

	slog.Info("apply database ownership", slog.String("name", options.Name))
	_, grantUserError := p.dbConnection.Exec(context.Background(), fmt.Sprintf("ALTER DATABASE %s OWNER TO %s", options.Name, options.Name))
	if grantUserError != nil {
		return grantUserError
	}

	return nil
}

func (p *Provider) Destroy(options database.DestroyOptions) error {
	slog.Info("destroying database", slog.String("name", options.Name))
	_, dropDatabaseError := p.dbConnection.Exec(context.Background(), fmt.Sprintf("DROP DATABASE %q", options.Name))
	if dropDatabaseError != nil && !helper.IsNotExistsError(dropDatabaseError) {
		return dropDatabaseError
	}
	slog.Info("destroying user", slog.String("name", options.Name))
	_, dropUserError := p.dbConnection.Exec(context.Background(), fmt.Sprintf("DROP USER %q", options.Name))
	if dropUserError != nil && !helper.IsNotExistsError(dropUserError) {
		return dropUserError
	}

	return nil
}

func (p *Provider) Initialize(dsn string) error {
	p.dsn = dsn
	dbConnection, databaseConnectionError := pgx.Connect(context.Background(), dsn)
	if databaseConnectionError != nil {
		return databaseConnectionError
	}
	p.dbConnection = dbConnection
	return nil
}

func (p *Provider) Close() error {
	return p.dbConnection.Close(context.Background())
}

func (p *Provider) HealthCheck(ctx context.Context) error {
	return p.dbConnection.Ping(ctx)
}

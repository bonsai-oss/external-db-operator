package mysql

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"

	"external-db-operator/internal/database"
)

func init() {
	database.RegisterProvider("mysql", Provide)
}

func Provide() database.Provider {
	return &Provider{}
}

type Provider struct {
	dbConnection *sql.DB
	dsn          string
}

var _ database.Provider = &Provider{}

func (p *Provider) Initialize(dsn string) error {
	db, dbInitialisationError := sql.Open("mysql", dsn)
	if dbInitialisationError != nil {
		return dbInitialisationError
	}

	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	p.dbConnection = db
	p.dsn = dsn

	return nil
}

func (p *Provider) Apply(options database.CreateOptions) error {
	slog.Info("creating database", slog.String("name", options.Name))
	_, databaseCreateError := p.dbConnection.Exec("CREATE DATABASE IF NOT EXISTS " + options.Name)
	if databaseCreateError != nil {
		return databaseCreateError
	}

	// check if user exists
	var userExists bool
	checkUserError := p.dbConnection.QueryRow("SELECT EXISTS(SELECT 1 FROM mysql.user WHERE user = ?)", options.Name).Scan(&userExists)
	if checkUserError != nil {
		return checkUserError
	}

	if userExists {
		slog.Info("alter user", slog.String("name", options.Name))
		if _, alterUserError := p.dbConnection.Exec("ALTER USER " + options.Name + " IDENTIFIED BY '" + options.Password + "'"); alterUserError != nil {
			return alterUserError
		}
	} else {
		slog.Info("create user", slog.String("name", options.Name))
		if _, createUserError := p.dbConnection.Exec("CREATE USER IF NOT EXISTS " + options.Name + " IDENTIFIED BY '" + options.Password + "'"); createUserError != nil {
			return createUserError
		}
	}

	slog.Info("apply database ownership", slog.String("name", options.Name))
	if _, grantPrivileges := p.dbConnection.Exec("GRANT ALL PRIVILEGES ON " + options.Name + ".* TO '" + options.Name + "'"); grantPrivileges != nil {
		return grantPrivileges
	}

	return nil
}

func (p *Provider) Destroy(options database.DestroyOptions) error {
	slog.Info("destroying database", slog.String("name", options.Name))
	_, dbDestroyError := p.dbConnection.Exec("DROP DATABASE IF EXISTS " + options.Name)
	if dbDestroyError != nil {
		return dbDestroyError
	}

	slog.Info("destroying user", slog.String("name", options.Name))
	_, userDestroyError := p.dbConnection.Exec("DROP USER IF EXISTS " + options.Name)
	if userDestroyError != nil {
		return userDestroyError
	}

	return nil
}

func (p *Provider) GetConnectionInfo() (database.ConnectionInfo, error) {
	config, dsnParseError := mysql.ParseDSN(p.dsn)
	if dsnParseError != nil {
		return database.ConnectionInfo{}, dsnParseError
	}

	host, port, splitAddressError := net.SplitHostPort(config.Addr)
	if splitAddressError != nil {
		return database.ConnectionInfo{}, splitAddressError
	}
	portInt, _ := strconv.Atoi(port)

	return database.ConnectionInfo{
		Host: host,
		Port: uint16(portInt),
	}, nil
}

func (p *Provider) HealthCheck(ctx context.Context) error {
	return p.dbConnection.PingContext(ctx)
}

func (p *Provider) Close() error {
	return p.dbConnection.Close()
}

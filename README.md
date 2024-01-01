# External-DB-Operator

The external db operator connects to an out of cluster database and manages the lifecycle of the database.

Operator Features:
- Management of database side spaces and user accounts
- Exposing database details as kubernetes secret

---

Supported Databases:
- PostgreSQL (via [pgx](https://github.com/jackc/pgx))

Other databases can be added by implementing the [Provider](internal/database/database.go) interface.

## Usage

Once the operator is reployed to the cluster, it will start watching for `bonsai-oss.org/v1/database` resources in all namespaces.

The name of the operator is specified via the `--instance-name` / `-i` flag and the used database provider in pattern `<provider>-<instance-name>`. An example for PostgreSQL would be `postgres-default`.\
That name is used to select the operator instance responsible for a specific database resource and can be specified via the `bonsai-oss.org/external-db-operator` label. See [manifests/test-database.yaml](manifests/test-database.yaml) for an example.

After creating the database resources, the operator will create a secret containing the database connection details (database, host, port, username, password) in the same namespace as the database resource.
It is named with the pattern `<secret_prefix>-<resource_name>` (e.a. `edb-your-database`). 

### Parameters

| Parameter                                         | Description                                                                                    | Default                                              |
|---------------------------------------------------|------------------------------------------------------------------------------------------------|------------------------------------------------------|
| `-p`, `--database-provider`, `$DATABASE_PROVIDER` | Database provider to use.                                                                      | postgres                                             |
| `-d`, `--database-dsn`, `$DATABASE_DSN`           | The DSN to use for the database provider.<br/> Check the specific database libaray for format. | postgres://postgres:postgres@localhost:5432/postgres |
| `-i`, `--instance-name`, `$INSTANCE_NAME`         | Name of the operator instance                                                                  | default                                              |
| `-s`, `--secret-prefix`, `$SECRET_PREFIX`         | Prefix for the secret name                                                                     | edb                                                  |

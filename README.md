# External-DB-Operator

The external db operator connects to an out of cluster database and manages the lifecycle of the database.

Operator Features:
- Management of database side spaces and user accounts
- Exposing database details as kubernetes secret

---

Supported Databases:
- PostgreSQL

Other databases can be added by implementing the [Provider](internal/database/database.go) interface.

## Usage

Once the operator is reployed to the cluster, it will start watching for `bonsai-oss.org/v1/database` resources in all namespaces.

The name of the operator is specified via the `--instance-name` / `-i` flag and the used database provider in pattern `<provider>-<instance-name>`. An example for PostgreSQL would be `postgres-default`.\
That name is used to select the operator instance responsible for a specific database resource and can be specified via the `bonsai-oss.org/external-db-operator` label. See [manifests/test-database.yaml](manifests/test-database.yaml) for an example.

After creating the database resources, the operator will create a secret containing the database connection details (database, host, port, username, password) in the same namespace as the database resource.

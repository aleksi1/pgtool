# pgtool

PostgreSQL backup and restore tool

## Build

```
go build -ldflags="-s -w" -o pgtool pgtool.go
```

## Backup with gzip

```
export PGPASSWORD="mypassword"
./pgtool backup \
  -db mydatabase \
  -user postgres \
  -host localhost \
  -backup-dir /var/backups/postgresql \
  -log-file /var/log/postgres_backup.log \
  -retention 7
```

This creates:
```
/var/backups/postgresql/mydatabase_2025-08-09_114200.dump.gz
```

## Restore from gzip

```
export PGPASSWORD="mypassword"
./pgtool restore \
  -db mydatabase \
  -user postgres \
  -host localhost \
  -file /var/backups/postgresql/mydatabase_2025-08-09_114200.dump.gz \
  -log-file /var/log/postgres_backup.log
```
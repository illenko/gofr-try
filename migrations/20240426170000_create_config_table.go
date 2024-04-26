package migrations

import "gofr.dev/pkg/gofr/migration"

const createTable = `CREATE TABLE IF NOT EXISTS config
(
    id             uuid PRIMARY KEY,
    title          varchar(256),
    skin           varchar(256)
);`

func createTableConfig() migration.Migrate {
	return migration.Migrate{
		UP: func(d migration.Datasource) error {
			_, err := d.SQL.Exec(createTable)
			if err != nil {
				return err
			}
			return nil
		},
	}
}

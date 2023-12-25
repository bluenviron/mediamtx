package conf

type Database struct {
	Use            bool   `json:"use"`
	DbAddress      string `json:"dbAddress"`
	DbPort         int    `json:"dbPort"`
	DbName         string `json:"dbName"`
	DbUser         string `json:"dbUser"`
	DbPassword     string `json:"dbPassword"`
	MaxConnections int    `json:"maxConnections"`
	Sql            Sql    `json:"sql"`
}

type Sql struct {
	InsertPath string `json:"insertPath"`
	UpdateSize string `json:"updateSize"`
}

func (db *Database) setDefaults() {

	db.Use = false
	db.DbAddress = "127.0.0.1"
	db.DbPort = 5432
	db.DbName = "postgres"
	db.DbUser = "postgres"
	db.DbPassword = ""
	db.MaxConnections = 0
	db.Sql = Sql{
		InsertPath: "",
		UpdateSize: "",
	}

}

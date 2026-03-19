module veilkey-vaultcenter

go 1.25.0

require (
	github.com/mattn/go-sqlite3 v1.14.33
	golang.org/x/crypto v0.49.0
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.1
)

require (
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/veilkey/veilkey-go-package v0.2.0 // indirect
	github.com/wneessen/go-mail v0.7.2 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/term v0.41.0 // indirect
	golang.org/x/text v0.35.0 // indirect
)

replace github.com/mattn/go-sqlite3 v1.14.33 => github.com/mutecomm/go-sqlcipher/v4 v4.4.2

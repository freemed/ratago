module github.com/freemed/ratago/xslt

go 1.24

toolchain go1.24.3

replace (
	github.com/freemed/gokogiri => ../../gokogiri
	github.com/freemed/gokogiri/help => ../../gokogiri/help
	github.com/freemed/gokogiri/util => ../../gokogiri/util
	github.com/freemed/gokogiri/xml => ../../gokogiri/xml
	github.com/freemed/gokogiri/xpath => ../../gokogiri/xpath
	github.com/freemed/ratago => ../
)

require (
	github.com/freemed/gokogiri/xml v0.0.0-20250831182455-de8ad4878374
	github.com/freemed/gokogiri/xpath v0.0.0-20250831182455-de8ad4878374
)

require (
	github.com/freemed/gokogiri/help v0.0.0-20250831182455-de8ad4878374 // indirect
	github.com/freemed/gokogiri/util v0.0.0-20250831182455-de8ad4878374 // indirect
)

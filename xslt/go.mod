module github.com/freemed/ratago/xslt

go 1.18

replace (
	github.com/freemed/gokogiri => ../../gokogiri
	github.com/freemed/gokogiri/help => ../../gokogiri/help
	github.com/freemed/gokogiri/util => ../../gokogiri/util
	github.com/freemed/gokogiri/xml => ../../gokogiri/xml
	github.com/freemed/gokogiri/xpath => ../../gokogiri/xpath
)

require (
	github.com/freemed/gokogiri/xml v0.0.0-00010101000000-000000000000
	github.com/freemed/gokogiri/xpath v0.0.0-00010101000000-000000000000
)

require (
	github.com/freemed/gokogiri/help v0.0.0-00010101000000-000000000000 // indirect
	github.com/freemed/gokogiri/util v0.0.0-00010101000000-000000000000 // indirect
)

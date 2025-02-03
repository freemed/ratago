module github.com/freemed/ratago/xslt

go 1.22

toolchain go1.23.2

replace (
	github.com/freemed/gokogiri => ../../gokogiri
	github.com/freemed/gokogiri/help => ../../gokogiri/help
	github.com/freemed/gokogiri/util => ../../gokogiri/util
	github.com/freemed/gokogiri/xml => ../../gokogiri/xml
	github.com/freemed/gokogiri/xpath => ../../gokogiri/xpath
	github.com/freemed/ratago => ../
)

require (
	github.com/freemed/gokogiri/xml v0.0.0-20230628164547-0f93de0487ac
	github.com/freemed/gokogiri/xpath v0.0.0-20230628164547-0f93de0487ac
)

require (
	github.com/freemed/gokogiri/help v0.0.0-20230628164547-0f93de0487ac // indirect
	github.com/freemed/gokogiri/util v0.0.0-20230628164547-0f93de0487ac // indirect
)

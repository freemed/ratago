module github.com/freemed/ratago/xslt

go 1.25.0

replace (
	github.com/freemed/gokogiri => ../../gokogiri
	github.com/freemed/gokogiri/help => ../../gokogiri/help
	github.com/freemed/gokogiri/util => ../../gokogiri/util
	github.com/freemed/gokogiri/xml => ../../gokogiri/xml
	github.com/freemed/gokogiri/xpath => ../../gokogiri/xpath
	github.com/freemed/ratago => ../
)

require (
	github.com/freemed/gokogiri/xml v0.0.0-20260810175053-c72f08123335
	github.com/freemed/gokogiri/xpath v0.0.0-20260810175053-c72f08123335
)

require (
	github.com/freemed/gokogiri/help v0.0.0-20260810175053-c72f08123335 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

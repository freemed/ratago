module github.com/freemed/ratago

go 1.18

replace (
	github.com/freemed/gokogiri/help => ../gokogiri/help
	github.com/freemed/gokogiri/html => ../gokogiri/html
	github.com/freemed/gokogiri/xml => ../gokogiri/xml
	github.com/freemed/gokogiri/xpath => ../gokogiri/xpath
	github.com/freemed/ratago/xslt => ./xslt
)

require (
	github.com/freemed/gokogiri/html v0.0.2
	github.com/freemed/gokogiri/xml v0.0.0-20220627154600-2acb041aa5ac
	github.com/freemed/kowhai v0.0.0-20150515033021-5b6ea3150fcb
	github.com/freemed/ratago/xslt v0.0.0-20220610164841-0ab820da5118
	github.com/smartystreets/goconvey v1.6.4
)

require (
	github.com/freemed/gokogiri/help v0.0.0-20220627154600-2acb041aa5ac // indirect
	github.com/freemed/gokogiri/util v0.0.0-20220627154600-2acb041aa5ac // indirect
	github.com/freemed/gokogiri/xpath v0.0.0-20220627154600-2acb041aa5ac // indirect
)

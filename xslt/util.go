package xslt

import (
	"github.com/freemed/gokogiri/xml"
	"os"
)

func xmlReadFile(filename string) (doc *xml.XmlDocument, err error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	doc, err = xml.Parse(data, xml.DefaultEncodingBytes, nil, xml.StrictParseOption, xml.DefaultEncodingBytes)
	return
}

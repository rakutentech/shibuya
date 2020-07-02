package controller

import (
	"errors"

	etree "github.com/beevik/etree"
)

func GetThreadGroups(planDoc *etree.Document) ([]*etree.Element, error) {
	jtp := planDoc.SelectElement("jmeterTestPlan")
	if jtp == nil {
		return nil, errors.New("Missing Jmeter Test plan in jmx")
	}
	ht := jtp.SelectElement("hashTree")
	if ht == nil {
		return nil, errors.New("Missing hash tree inside Jmeter test plan in jmx")
	}
	ht = ht.SelectElement("hashTree")
	if ht == nil {
		return nil, errors.New("Missing hash tree inside hash tree in jmx")
	}
	tgs := ht.SelectElements("ThreadGroup")
	stgs := ht.SelectElements("SetupThreadGroup")
	tgs = append(tgs, stgs...)
	return tgs, nil
}

func ParseTestPlan(path string) (*etree.Document, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromFile(path); err != nil {
		return nil, err
	}
	return doc, nil
}

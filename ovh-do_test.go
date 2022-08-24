package main

import (
	"testing"
	"fmt"
)

func TestSplitImgName(t *testing.T) {
	doTests(t, []test{
		{
			"`` -> error",
			splitImgName,
			[]interface{}{""},
			[]interface{}{
				"", -1., "", fmt.Errorf("Invalid version name: ''"),
			},
		},
		{
			"No version number",
			splitImgName,
			[]interface{}{"Debian"},
			[]interface{}{
				"", -1., "", fmt.Errorf("Invalid version name: 'Debian'"),
			},
		},
		{
			"No extra, integer version number",
			splitImgName,
			[]interface{}{"Debian 11"},
			[]interface{}{
				"Debian", 11., "", nil,
			},
		},
		{
			"No extra, non-integer version number",
			splitImgName,
			[]interface{}{"Ubuntu 20.04"},
			[]interface{}{
				"Ubuntu", 20.04, "", nil,
			},
		},
		{
			"With extra",
			splitImgName,
			[]interface{}{"Debian 10 - Docker"},
			[]interface{}{
				"Debian", 10., " - Docker", nil,
			},
		},
		{
			"No-extra, multi words distribution name",
			splitImgName,
			[]interface{}{"Rocky Linux 8"},
			[]interface{}{
				"Rocky Linux", 8., "", nil,
			},
		},
	})
}

func TestIsImgId(t *testing.T) {
	doTests(t, []test{
		{
			"`` -> no",
			isImgId,
			[]interface{}{""},
			[]interface{}{false},
		},
		{
			"one number-off -> no",
			isImgId,
			[]interface{}{"4b12e37-4241-4301-aadf-85ae34cdd6a9"},
			[]interface{}{false},
		},
		{
			"correct image id",
			isImgId,
			[]interface{}{"f4b12e37-4241-4301-aadf-85ae34cdd6a9"},
			[]interface{}{true},
		},
	})
}

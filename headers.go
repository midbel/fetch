package fetch

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

var ErrSyntax = errors.New("syntax error")

type Accept struct {
	Type string
	Pref float64
}

func ParseAccept(str string) ([]Accept, error) {
	var as []Accept
	for _, str := range strings.Split(str, ",") {
		a, err := parseAccept(strings.TrimSpace(str))
		if err != nil {
			return nil, err
		}
		as = append(as, a)
	}
	sort.Slice(as, func(i, j int) bool {
		return as[i].Pref > as[j].Pref
	})
	return as, nil
}

type Link struct {
	URL    string
	Rel    string
	Target string
	Title  string
	Media  string
	Lang   string
	Type   string
}

func ParseLink(str string) ([]Link, error) {
	var is []Link
	for _, str := range strings.Split(str, ",") {
		i, err := parseLink(strings.TrimSpace(str))
		if err != nil {
			return nil, err
		}
		is = append(is, i)
	}
	return is, nil
}

func parseAccept(str string) (Accept, error) {
	var a Accept
	a.Pref = 1
	a.Type, str = splitParams(str)
	for _, str := range strings.Split(str, ";") {
		name, value, err := splitKeyValue(str)
		if err != nil {
			return a, err
		}
		if name == "q" {
			a.Pref, err = strconv.ParseFloat(value, 64)
			if err != nil {
				return a, ErrSyntax
			}
		}
	}
	return a, nil
}

func parseLink(str string) (Link, error) {
	var i Link
	i.URL, str = splitParams(str)
	if i.URL[0] == '<' && i.URL[len(i.URL)-1] == '>' {
		i.URL = i.URL[1 : len(i.URL)-1]
	} else {
		return i, ErrSyntax
	}
	for _, str := range strings.Split(str, ";") {
		name, value, err := splitKeyValue(str)
		if err != nil {
			return i, err
		}
		switch name {
		case "rel":
			i.Rel = strings.Trim(value, "\"")
		case "target":
			i.Target = strings.Trim(value, "\"")
		case "title":
			i.Title = strings.Trim(value, "\"")
		case "media":
			i.Media = strings.Trim(value, "\"")
		case "type":
			i.Type = strings.Trim(value, "\"")
		case "lang":
			i.Lang = strings.Trim(value, "\"")
		}
	}
	return i, nil
}

func splitKeyValue(str string) (string, string, error) {
	str = strings.TrimSpace(str)
	x := strings.Index(str, "=")
	if x <= 0 {
		return "", "", ErrSyntax
	}
	return strings.TrimSpace(str[:x]), strings.TrimSpace(str[x+1:]), nil
}

func splitParams(str string) (string, string) {
	x := strings.Index(str, ";")
	if x <= 0 {
		return str, ""
	}
	return strings.TrimSpace(str[:x]), strings.TrimSpace(str[x+1:])
}

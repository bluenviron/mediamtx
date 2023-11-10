package record

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

func leadingZeros(v int, size int) string {
	out := strconv.FormatInt(int64(v), 10)
	if len(out) >= size {
		return out
	}

	out2 := ""
	for i := 0; i < (size - len(out)); i++ {
		out2 += "0"
	}

	return out2 + out
}

type recordPathParams struct {
	path string
	time time.Time
}

func decodeRecordPath(format string, v string) *recordPathParams {
	re := format

	for _, ch := range []uint8{
		'\\',
		'.',
		'+',
		'*',
		'?',
		'^',
		'$',
		'(',
		')',
		'[',
		']',
		'{',
		'}',
		'|',
	} {
		re = strings.ReplaceAll(re, string(ch), "\\"+string(ch))
	}

	re = strings.ReplaceAll(re, "%path", "(.*?)")
	re = strings.ReplaceAll(re, "%Y", "([0-9]{4})")
	re = strings.ReplaceAll(re, "%m", "([0-9]{2})")
	re = strings.ReplaceAll(re, "%d", "([0-9]{2})")
	re = strings.ReplaceAll(re, "%H", "([0-9]{2})")
	re = strings.ReplaceAll(re, "%M", "([0-9]{2})")
	re = strings.ReplaceAll(re, "%S", "([0-9]{2})")
	re = strings.ReplaceAll(re, "%f", "([0-9]{6})")
	r := regexp.MustCompile(re)

	var groupMapping []string
	cur := format
	for {
		i := strings.Index(cur, "%")
		if i < 0 {
			break
		}

		cur = cur[i:]

		for _, va := range []string{
			"%path",
			"%Y",
			"%m",
			"%d",
			"%H",
			"%M",
			"%S",
			"%f",
		} {
			if strings.HasPrefix(cur, va) {
				groupMapping = append(groupMapping, va)
			}
		}

		cur = cur[1:]
	}

	matches := r.FindStringSubmatch(v)
	if matches == nil {
		return nil
	}

	values := make(map[string]string)

	for i, match := range matches[1:] {
		values[groupMapping[i]] = match
	}

	var year int
	var month time.Month = 1
	day := 1
	var hour int
	var minute int
	var second int
	var micros int

	for k, v := range values {
		switch k {
		case "%Y":
			tmp, _ := strconv.ParseInt(v, 10, 64)
			year = int(tmp)

		case "%m":
			tmp, _ := strconv.ParseInt(v, 10, 64)
			month = time.Month(int(tmp))

		case "%d":
			tmp, _ := strconv.ParseInt(v, 10, 64)
			day = int(tmp)

		case "%H":
			tmp, _ := strconv.ParseInt(v, 10, 64)
			hour = int(tmp)

		case "%M":
			tmp, _ := strconv.ParseInt(v, 10, 64)
			minute = int(tmp)

		case "%S":
			tmp, _ := strconv.ParseInt(v, 10, 64)
			second = int(tmp)

		case "%f":
			tmp, _ := strconv.ParseInt(v, 10, 64)
			micros = int(tmp)
		}
	}

	t := time.Date(year, month, day, hour, minute, second, micros*1000, time.Local)

	return &recordPathParams{
		path: values["%path"],
		time: t,
	}
}

func encodeRecordPath(params *recordPathParams, v string) string {
	v = strings.ReplaceAll(v, "%Y", strconv.FormatInt(int64(params.time.Year()), 10))
	v = strings.ReplaceAll(v, "%m", leadingZeros(int(params.time.Month()), 2))
	v = strings.ReplaceAll(v, "%d", leadingZeros(params.time.Day(), 2))
	v = strings.ReplaceAll(v, "%H", leadingZeros(params.time.Hour(), 2))
	v = strings.ReplaceAll(v, "%M", leadingZeros(params.time.Minute(), 2))
	v = strings.ReplaceAll(v, "%S", leadingZeros(params.time.Second(), 2))
	v = strings.ReplaceAll(v, "%f", leadingZeros(params.time.Nanosecond()/1000, 6))
	return v
}

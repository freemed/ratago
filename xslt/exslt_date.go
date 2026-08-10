package xslt

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/freemed/gokogiri/xml"
	"github.com/freemed/gokogiri/xpath"
)

// ---------- EXSLT Date/Time types ----------

type exsltDateTime struct {
	Year   int
	Month  int // 1-12
	Day    int // 1-31
	Hour   int // 0-23
	Minute int // 0-59
	Second int // 0-59
	Nano   int // fractional seconds as nanoseconds

	HasTime bool // false if only date was given
	HasDay  bool // false if only YYYY-MM was given

	TZOffset int // timezone offset in seconds (0 = UTC, negative = west)
	HasTZ    bool
}

type exsltDuration struct {
	Negative bool

	Years   int
	Months  int
	Days    int
	Hours   int
	Minutes int
	Seconds float64 // can have fractional part
}

// ---------- Parsing ----------

// parseEXSLTDateTime parses an ISO 8601 date-time string as accepted by EXSLT.
// Formats:
//
//	YYYY-MM-DDThh:mm:ss.sssZ
//	YYYY-MM-DDThh:mm:ss.sss+HH:MM
//	YYYY-MM-DDThh:mm:ss
//	YYYY-MM-DD
//	YYYY-MM
func parseEXSLTDateTime(s string) (exsltDateTime, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return exsltDateTime{}, false
	}

	dt := exsltDateTime{}
	rest := s

	// Check for leading minus (BC dates - we don't support those but parse them)
	if strings.HasPrefix(rest, "-") {
		dt.Year = -1 // signal BC; don't parse further
		rest = rest[1:]
	}

	// Parse YYYY[-MM[-DD]]
	if len(rest) < 4 {
		return exsltDateTime{}, false
	}
	year, err := strconv.Atoi(rest[:4])
	if err != nil {
		return exsltDateTime{}, false
	}
	if strings.HasPrefix(s, "-") {
		year = -year
	}
	dt.Year = year
	rest = rest[4:]

	if len(rest) == 0 {
		// Just a year - not standard but handle gracefully
		dt.Month = 1
		dt.Day = 1
		return dt, true
	}

	if rest[0] != '-' {
		return exsltDateTime{}, false
	}
	rest = rest[1:] // skip '-'

	if len(rest) < 2 {
		return exsltDateTime{}, false
	}
	month, err := strconv.Atoi(rest[:2])
	if err != nil || month < 1 || month > 12 {
		return exsltDateTime{}, false
	}
	dt.Month = month
	rest = rest[2:]

	if len(rest) == 0 {
		// YYYY-MM format
		dt.Day = 1
		dt.HasDay = false
		return dt, true
	}

	if rest[0] != '-' {
		// No day separator - could be YYYY-MM directly followed by T
		if rest[0] == 'T' {
			dt.Day = 1
			dt.HasDay = false
			// fall through to time parsing
		} else {
			return exsltDateTime{}, false
		}
	} else {
		rest = rest[1:] // skip '-'
		if len(rest) < 2 {
			return exsltDateTime{}, false
		}
		day, err := strconv.Atoi(rest[:2])
		if err != nil || day < 1 || day > 31 {
			return exsltDateTime{}, false
		}
		dt.Day = day
		dt.HasDay = true
		rest = rest[2:]
	}

	// Check for time component
	if len(rest) == 0 {
		return dt, true
	}
	if rest[0] != 'T' {
		// Check for timezone directly on date
		if rest[0] == 'Z' || rest[0] == '+' || rest[0] == '-' {
			return dt, true // ignore timezone on date-only for now
		}
		return dt, true
	}
	rest = rest[1:] // skip 'T'

	// Parse hh[:mm[:ss[.sss]]]
	if len(rest) < 2 {
		return exsltDateTime{}, false
	}
	hour, err := strconv.Atoi(rest[:2])
	if err != nil || hour < 0 || hour > 23 {
		return exsltDateTime{}, false
	}
	dt.Hour = hour
	dt.HasTime = true
	rest = rest[2:]

	if len(rest) == 0 {
		return dt, true
	}
	if rest[0] != ':' {
		return dt, true
	}
	rest = rest[1:] // skip ':'

	if len(rest) < 2 {
		return exsltDateTime{}, false
	}
	minute, err := strconv.Atoi(rest[:2])
	if err != nil || minute < 0 || minute > 59 {
		return exsltDateTime{}, false
	}
	dt.Minute = minute
	rest = rest[2:]

	if len(rest) == 0 {
		return dt, true
	}
	if rest[0] != ':' {
		// Check for timezone
		return parseTZ(&dt, rest)
	}
	rest = rest[1:] // skip ':'

	if len(rest) < 2 {
		return exsltDateTime{}, false
	}
	sec, err := strconv.Atoi(rest[:2])
	if err != nil || sec < 0 || sec > 59 {
		return exsltDateTime{}, false
	}
	dt.Second = sec
	rest = rest[2:]

	// Optional fractional seconds
	if len(rest) > 0 && rest[0] == '.' {
		rest = rest[1:]
		j := 0
		for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
			j++
		}
		if j > 0 {
			frac := rest[:j]
			// Pad/truncate to nanosecond precision
			for len(frac) < 9 {
				frac += "0"
			}
			if len(frac) > 9 {
				frac = frac[:9]
			}
			dt.Nano, _ = strconv.Atoi(frac)
			rest = rest[j:]
		}
	}

	if len(rest) == 0 {
		return dt, true
	}
	return parseTZ(&dt, rest)
}

func parseTZ(dt *exsltDateTime, s string) (exsltDateTime, bool) {
	if s[0] == 'Z' {
		dt.TZOffset = 0
		dt.HasTZ = true
		return *dt, true
	}
	if s[0] == '+' || s[0] == '-' {
		sign := 1
		if s[0] == '-' {
			sign = -1
		}
		rest := s[1:]
		if len(rest) < 2 {
			return *dt, false
		}
		hh, _ := strconv.Atoi(rest[:2])
		mm := 0
		rest = rest[2:]
		if len(rest) >= 1 && rest[0] == ':' {
			rest = rest[1:]
			if len(rest) >= 2 {
				mm, _ = strconv.Atoi(rest[:2])
			}
		}
		dt.TZOffset = sign * (hh*3600 + mm*60)
		dt.HasTZ = true
		return *dt, true
	}
	return *dt, true
}

// parseEXSLTDuration parses an ISO 8601 duration: P[nY][nM][nD][T[nH][nM][nS]]
// Seconds may have a fractional part (e.g., PT1.5S).
func parseEXSLTDuration(s string) (exsltDuration, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return exsltDuration{}, false
	}

	dur := exsltDuration{}

	if s[0] == '-' {
		dur.Negative = true
		s = s[1:]
	}
	if len(s) == 0 || s[0] != 'P' {
		return exsltDuration{}, false
	}
	s = s[1:]

	if len(s) == 0 {
		return dur, true
	}

	// Split at T
	var datePart, timePart string
	if idx := strings.IndexByte(s, 'T'); idx >= 0 {
		datePart = s[:idx]
		timePart = s[idx+1:]
	} else {
		datePart = s
	}

	// Parse date part
	if datePart != "" {
		for len(datePart) > 0 {
			// Find next letter
			idx := strings.IndexFunc(datePart, func(r rune) bool {
				return r == 'Y' || r == 'M' || r == 'D'
			})
			if idx < 0 {
				break
			}
			val, _ := strconv.Atoi(datePart[:idx])
			switch datePart[idx] {
			case 'Y':
				dur.Years = val
			case 'M':
				dur.Months = val
			case 'D':
				dur.Days = val
			}
			datePart = datePart[idx+1:]
		}
	}

	// Parse time part
	if timePart != "" {
		for len(timePart) > 0 {
			idx := strings.IndexFunc(timePart, func(r rune) bool {
				return r == 'H' || r == 'M' || r == 'S'
			})
			if idx < 0 {
				break
			}
			valStr := timePart[:idx]
			val, fracErr := strconv.ParseFloat(valStr, 64)
			switch timePart[idx] {
			case 'H':
				dur.Hours = int(val)
			case 'M':
				dur.Minutes = int(val)
			case 'S':
				dur.Seconds = val
				// If int part is separate from fractional
				if fracErr != nil {
					dur.Seconds = 0 // fallback
				}
			}
			timePart = timePart[idx+1:]
		}
	}

	return dur, true
}

// ---------- Formatting ----------

func (dt exsltDateTime) formatDate() string {
	if dt.HasDay {
		return fmt.Sprintf("%04d-%02d-%02d", dt.Year, dt.Month, dt.Day)
	}
	if dt.Month > 0 {
		return fmt.Sprintf("%04d-%02d", dt.Year, dt.Month)
	}
	return fmt.Sprintf("%04d", dt.Year)
}

func (dt exsltDateTime) formatDateTime() string {
	date := dt.formatDate()
	if !dt.HasTime {
		return date
	}
	tzStr := ""
	if dt.HasTZ {
		if dt.TZOffset == 0 {
			tzStr = "Z"
		} else {
			sign := "+"
			offset := dt.TZOffset
			if offset < 0 {
				sign = "-"
				offset = -offset
			}
			hh := offset / 3600
			mm := (offset % 3600) / 60
			tzStr = fmt.Sprintf("%s%02d:%02d", sign, hh, mm)
		}
	}
	if dt.Nano > 0 {
		frac := fmt.Sprintf("%09d", dt.Nano)
		frac = strings.TrimRight(frac, "0")
		return fmt.Sprintf("%sT%02d:%02d:%02d.%s%s",
			date, dt.Hour, dt.Minute, dt.Second, frac, tzStr)
	}
	return fmt.Sprintf("%sT%02d:%02d:%02d%s",
		date, dt.Hour, dt.Minute, dt.Second, tzStr)
}

func (dt exsltDateTime) formatTime() string {
	if !dt.HasTime {
		return "00:00:00"
	}
	tzStr := ""
	if dt.HasTZ {
		if dt.TZOffset == 0 {
			tzStr = "Z"
		} else {
			sign := "+"
			offset := dt.TZOffset
			if offset < 0 {
				sign = "-"
				offset = -offset
			}
			hh := offset / 3600
			mm := (offset % 3600) / 60
			tzStr = fmt.Sprintf("%s%02d:%02d", sign, hh, mm)
		}
	}
	if dt.Nano > 0 {
		frac := fmt.Sprintf("%09d", dt.Nano)
		frac = strings.TrimRight(frac, "0")
		return fmt.Sprintf("%02d:%02d:%02d.%s%s",
			dt.Hour, dt.Minute, dt.Second, frac, tzStr)
	}
	return fmt.Sprintf("%02d:%02d:%02d%s",
		dt.Hour, dt.Minute, dt.Second, tzStr)
}

func (d exsltDuration) format() string {
	var sb strings.Builder
	if d.Negative {
		sb.WriteByte('-')
	}
	sb.WriteByte('P')
	hasDate := d.Years > 0 || d.Months > 0 || d.Days > 0
	hasTime := d.Hours > 0 || d.Minutes > 0 || d.Seconds > 0

	if d.Years > 0 {
		sb.WriteString(fmt.Sprintf("%dY", d.Years))
	}
	if d.Months > 0 {
		sb.WriteString(fmt.Sprintf("%dM", d.Months))
	}
	if d.Days > 0 {
		sb.WriteString(fmt.Sprintf("%dD", d.Days))
	}
	if !hasDate && !hasTime {
		// zero duration
		sb.WriteString("T0S")
		return sb.String()
	}
	if hasTime {
		sb.WriteByte('T')
		if d.Hours > 0 {
			sb.WriteString(fmt.Sprintf("%dH", d.Hours))
		}
		if d.Minutes > 0 {
			sb.WriteString(fmt.Sprintf("%dM", d.Minutes))
		}
		if d.Seconds > 0 {
			sb.WriteString(strconv.FormatFloat(d.Seconds, 'f', -1, 64))
			sb.WriteByte('S')
		}
	}
	return sb.String()
}

// ---------- Arithmetic ----------

func daysInMonth(year, month int) int {
	if month == 2 {
		if (year%4 == 0 && year%100 != 0) || (year%400 == 0) {
			return 29
		}
		return 28
	}
	if month == 4 || month == 6 || month == 9 || month == 11 {
		return 30
	}
	return 31
}

// addDuration adds a duration to a date-time, following EXSLT semantics.
func addDuration(dt exsltDateTime, d exsltDuration) exsltDateTime {
	if d.Negative {
		// Negate the duration
		d.Negative = false
		d.Years = -d.Years
		d.Months = -d.Months
		d.Days = -d.Days
		d.Hours = -d.Hours
		d.Minutes = -d.Minutes
		d.Seconds = -d.Seconds
	}

	// Add months first, clamping day as needed
	dt.Month += d.Months
	for dt.Month > 12 {
		dt.Year++
		dt.Month -= 12
	}
	for dt.Month < 1 {
		dt.Year--
		dt.Month += 12
	}
	dt.Year += d.Years

	// Clamp day to days in month
	dim := daysInMonth(dt.Year, dt.Month)
	if dt.Day > dim {
		dt.Day = dim
	}

	// Add days, rolling through months
	totalDays := dt.Day + d.Days
	for totalDays > daysInMonth(dt.Year, dt.Month) {
		totalDays -= daysInMonth(dt.Year, dt.Month)
		dt.Month++
		if dt.Month > 12 {
			dt.Month = 1
			dt.Year++
		}
	}
	dt.Day = totalDays
	dt.HasDay = true

	// Add time
	if dt.HasTime || d.Hours != 0 || d.Minutes != 0 || d.Seconds != 0 {
		dt.HasTime = true
		totalSec := dt.Hour*3600 + dt.Minute*60 + dt.Second
		totalSec += d.Hours*3600 + d.Minutes*60 + int(d.Seconds)
		fracSec := d.Seconds - float64(int(d.Seconds))
		dt.Nano += int(fracSec * 1e9)
		for dt.Nano >= 1e9 {
			dt.Nano -= 1e9
			totalSec++
		}
		for dt.Nano < 0 {
			dt.Nano += 1e9
			totalSec--
		}

		// Roll seconds into days
		for totalSec < 0 {
			totalSec += 86400
			dt.Day--
			if dt.Day < 1 {
				dt.Month--
				if dt.Month < 1 {
					dt.Month = 12
					dt.Year--
				}
				dt.Day = daysInMonth(dt.Year, dt.Month)
			}
		}
		for totalSec >= 86400 {
			totalSec -= 86400
			dt.Day++
			if dt.Day > daysInMonth(dt.Year, dt.Month) {
				dt.Day = 1
				dt.Month++
				if dt.Month > 12 {
					dt.Month = 1
					dt.Year++
				}
			}
		}

		dt.Hour = totalSec / 3600
		totalSec %= 3600
		dt.Minute = totalSec / 60
		dt.Second = totalSec % 60
	}

	return dt
}

// toGoTime converts an exsltDateTime to a Go time.Time.
func (dt exsltDateTime) toGoTime() time.Time {
	loc := time.UTC
	if dt.HasTZ {
		loc = time.FixedZone("", dt.TZOffset)
	}
	day := dt.Day
	if day < 1 {
		day = 1
	}
	return time.Date(dt.Year, time.Month(dt.Month), day,
		dt.Hour, dt.Minute, dt.Second, dt.Nano, loc)
}

// fromGoTime converts a Go time.Time to an exsltDateTime.
func fromGoTime(t time.Time, hasTime bool) exsltDateTime {
	_, offset := t.Zone()
	return exsltDateTime{
		Year:     t.Year(),
		Month:    int(t.Month()),
		Day:      t.Day(),
		Hour:     t.Hour(),
		Minute:   t.Minute(),
		Second:   t.Second(),
		Nano:     t.Nanosecond(),
		HasTime:  hasTime,
		HasDay:   true,
		TZOffset: offset,
		HasTZ:    true,
	}
}

// diffDateTime computes duration between two date-times (dt2 - dt1).
func diffDateTime(dt1, dt2 exsltDateTime) exsltDuration {
	t1 := dt1.toGoTime()
	t2 := dt2.toGoTime()

	diff := t2.Sub(t1)
	neg := diff < 0
	if neg {
		diff = -diff
	}

	seconds := diff.Seconds()
	return exsltDuration{
		Negative: neg,
		Seconds:  seconds,
	}
}

// durationToSeconds converts a duration to total seconds.
func durationToSeconds(d exsltDuration) float64 {
	const daysPerYear = 365.2425 // average
	const daysPerMonth = daysPerYear / 12
	total := d.Seconds
	total += float64(d.Minutes * 60)
	total += float64(d.Hours * 3600)
	total += float64(d.Days * 86400)
	total += float64(d.Months) * daysPerMonth * 86400
	total += float64(d.Years) * daysPerYear * 86400
	if d.Negative {
		total = -total
	}
	return total
}

// ---------- EXSLT Date/Time Functions ----------

func EXSLTdateDateTime(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) == 0 {
		// Return current date-time
		now := time.Now()
		return now.Format("2006-01-02T15:04:05-07:00")
	}
	s := argValToString(args[0])
	dt, ok := parseEXSLTDateTime(s)
	if !ok {
		return ""
	}
	return dt.formatDateTime()
}

func EXSLTdateDate(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return ""
	}
	s := argValToString(args[0])
	dt, ok := parseEXSLTDateTime(s)
	if !ok {
		return ""
	}
	return dt.formatDate()
}

func EXSLTdateTime(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return ""
	}
	s := argValToString(args[0])
	dt, ok := parseEXSLTDateTime(s)
	if !ok {
		return ""
	}
	return dt.formatTime()
}

func EXSLTdateYear(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	s := argValToString(args[0])
	dt, ok := parseEXSLTDateTime(s)
	if !ok {
		return nil
	}
	return float64(dt.Year)
}

func EXSLTdateMonthInYear(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	s := argValToString(args[0])
	dt, ok := parseEXSLTDateTime(s)
	if !ok {
		return nil
	}
	return float64(dt.Month)
}

func EXSLTdateDayInMonth(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	s := argValToString(args[0])
	dt, ok := parseEXSLTDateTime(s)
	if !ok {
		return nil
	}
	if !dt.HasDay {
		return math.NaN()
	}
	return float64(dt.Day)
}

func EXSLTdateDayInYear(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	s := argValToString(args[0])
	dt, ok := parseEXSLTDateTime(s)
	if !ok {
		return nil
	}
	t := dt.toGoTime()
	return float64(t.YearDay())
}

func EXSLTdateHourInDay(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	s := argValToString(args[0])
	dt, ok := parseEXSLTDateTime(s)
	if !ok || !dt.HasTime {
		return nil
	}
	return float64(dt.Hour)
}

func EXSLTdateMinuteInHour(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	s := argValToString(args[0])
	dt, ok := parseEXSLTDateTime(s)
	if !ok || !dt.HasTime {
		return nil
	}
	return float64(dt.Minute)
}

func EXSLTdateSecondInMinute(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	s := argValToString(args[0])
	dt, ok := parseEXSLTDateTime(s)
	if !ok || !dt.HasTime {
		return nil
	}
	if dt.Nano > 0 {
		return float64(dt.Second) + float64(dt.Nano)/1e9
	}
	return float64(dt.Second)
}

func EXSLTdateWeekInYear(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	s := argValToString(args[0])
	dt, ok := parseEXSLTDateTime(s)
	if !ok {
		return nil
	}
	t := dt.toGoTime()
	_, week := t.ISOWeek()
	return float64(week)
}

func EXSLTdateDayInWeek(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	s := argValToString(args[0])
	dt, ok := parseEXSLTDateTime(s)
	if !ok {
		return nil
	}
	t := dt.toGoTime()
	wd := t.Weekday()
	// ISO 8601: Monday=1, Sunday=7
	if wd == time.Sunday {
		return float64(7)
	}
	return float64(wd)
}

func EXSLTdateAdd(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 2 {
		return ""
	}
	dateStr := argValToString(args[0])
	durStr := argValToString(args[1])

	dt, ok := parseEXSLTDateTime(dateStr)
	if !ok {
		return ""
	}
	dur, ok := parseEXSLTDuration(durStr)
	if !ok {
		return ""
	}

	result := addDuration(dt, dur)
	return result.formatDateTime()
}

func EXSLTdateAddDuration(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 2 {
		return ""
	}
	d1Str := argValToString(args[0])
	d2Str := argValToString(args[1])

	d1, ok1 := parseEXSLTDuration(d1Str)
	d2, ok2 := parseEXSLTDuration(d2Str)
	if !ok1 || !ok2 {
		return ""
	}

	// Add durations
	s1 := durationToSeconds(d1)
	s2 := durationToSeconds(d2)
	total := s1 + s2

	result := exsltDuration{Seconds: total}
	if total < 0 {
		result.Negative = true
		result.Seconds = -total
	}
	return result.format()
}

func EXSLTdateDuration(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return ""
	}
	switch v := args[0].(type) {
	case float64:
		d := exsltDuration{}
		secs := v
		if secs < 0 {
			d.Negative = true
			secs = -secs
		}
		// Convert to nominal duration components
		d.Days = int(secs) / 86400
		rem := secs - float64(d.Days*86400)
		d.Hours = int(rem) / 3600
		rem -= float64(d.Hours * 3600)
		d.Minutes = int(rem) / 60
		d.Seconds = rem - float64(d.Minutes*60)
		return d.format()
	case []interface{}:
		s := argValToString(v)
		dur, ok := parseEXSLTDuration(s)
		if !ok {
			return ""
		}
		return dur.format()
	default:
		s := argValToString(v)
		dur, ok := parseEXSLTDuration(s)
		if !ok {
			return ""
		}
		return dur.format()
	}
}

func EXSLTdateSum(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return ""
	}
	nodes, ok := nodeSetFromPointers(args[0])
	if !ok {
		return ""
	}
	var total float64
	for _, p := range nodes {
		n := xml.NewNode(p.(*xml.InternalNode), nil)
		s := strings.TrimSpace(n.String())
		dur, ok := parseEXSLTDuration(s)
		if ok {
			total += durationToSeconds(dur)
		}
	}
	result := exsltDuration{Seconds: total}
	if total < 0 {
		result.Negative = true
		result.Seconds = -total
	}
	return result.format()
}

func EXSLTdateSeconds(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) < 1 {
		return nil
	}
	switch v := args[0].(type) {
	case []interface{}:
		// Sum seconds of all node values
		nodes, _ := nodeSetFromPointers(v)
		var total float64
		for _, p := range nodes {
			n := xml.NewNode(p.(*xml.InternalNode), nil)
			s := strings.TrimSpace(n.String())
			dur, ok := parseEXSLTDuration(s)
			if ok {
				total += durationToSeconds(dur)
			}
		}
		return total
	default:
		s := argValToString(v)
		dur, ok := parseEXSLTDuration(s)
		if !ok {
			return nil
		}
		return durationToSeconds(dur)
	}
}

func EXSLTdateDifference(context xpath.VariableScope, args []interface{}) interface{} {
	if len(args) != 2 {
		return ""
	}
	s1 := argValToString(args[0])
	s2 := argValToString(args[1])

	dt1, ok1 := parseEXSLTDateTime(s1)
	dt2, ok2 := parseEXSLTDateTime(s2)
	if !ok1 || !ok2 {
		return ""
	}

	dur := diffDateTime(dt1, dt2)
	return dur.format()
}

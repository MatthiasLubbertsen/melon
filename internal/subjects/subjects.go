// Package subjects maps Zermelo subject codes to readable names.
package subjects

import (
	"regexp"
	"strings"
)

// Name is a subject name in both supported languages.
type Name struct{ NL, EN string }

// Defaults covers the codes Dutch secondary schools commonly use. Schools are
// free to invent their own, which is why users can override every entry.
var Defaults = map[string]Name{
	"ak":   {"Aardrijkskunde", "Geography"},
	"ag":   {"Aardrijkskunde", "Geography"},
	"anw":  {"Algemene natuurwetenschappen", "General science"},
	"ar":   {"Arabisch", "Arabic"},
	"beco": {"Bedrijfseconomie", "Business economics"},
	"be":   {"Bedrijfseconomie", "Business economics"},
	"bg":   {"Begeleiding", "Guidance"},
	"bi":   {"Biologie", "Biology"},
	"bio":  {"Biologie", "Biology"},
	"bv":   {"Beeldende vorming", "Visual arts"},
	"bte":  {"Beeldende vorming", "Visual arts"},
	"bsm":  {"Bewegen, sport en maatschappij", "Sport and society"},
	"ch":   {"Chinees", "Chinese"},
	"ckv":  {"Culturele en kunstzinnige vorming", "Cultural and artistic education"},
	"dr":   {"Drama", "Drama"},
	"da":   {"Dans", "Dance"},
	"du":   {"Duits", "German"},
	"dutl": {"Duits", "German"},
	"ec":   {"Economie", "Economics"},
	"econ": {"Economie", "Economics"},
	"en":   {"Engels", "English"},
	"entl": {"Engels", "English"},
	"fa":   {"Frans", "French"},
	"fatl": {"Frans", "French"},
	"fi":   {"Filosofie", "Philosophy"},
	"fr":   {"Fries", "Frisian"},
	"gd":   {"Godsdienst", "Religious education"},
	"gds":  {"Godsdienst", "Religious education"},
	"gl":   {"Godsdienst en levensbeschouwing", "Religion and ethics"},
	"gr":   {"Grieks", "Greek"},
	"gtc":  {"Griekse taal en cultuur", "Greek language and culture"},
	"gs":   {"Geschiedenis", "History"},
	"ges":  {"Geschiedenis", "History"},
	"ha":   {"Handvaardigheid", "Crafts"},
	"hv":   {"Handvaardigheid", "Crafts"},
	"in":   {"Informatica", "Computer science"},
	"inf":  {"Informatica", "Computer science"},
	"it":   {"Italiaans", "Italian"},
	"kcv":  {"Klassieke culturele vorming", "Classical cultural education"},
	"ku":   {"Kunst", "Art"},
	"kua":  {"Kunst algemeen", "Art history"},
	"kubv": {"Kunst beeldende vorming", "Visual arts"},
	"kumu": {"Kunst muziek", "Music"},
	"kwt":  {"Keuzewerktijd", "Flex hour"},
	"la":   {"Latijn", "Latin"},
	"ltc":  {"Latijnse taal en cultuur", "Latin language and culture"},
	"lb":   {"Levensbeschouwing", "Ethics"},
	"lv":   {"Levensbeschouwing", "Ethics"},
	"lo":   {"Lichamelijke opvoeding", "Physical education"},
	"lob":  {"Loopbaanoriëntatie", "Career orientation"},
	"m&m":  {"Mens en maatschappij", "Humanities"},
	"mm":   {"Mens en maatschappij", "Humanities"},
	"m&n":  {"Mens en natuur", "Science"},
	"mn":   {"Mens en natuur", "Science"},
	"ma":   {"Maatschappijleer", "Social studies"},
	"maat": {"Maatschappijleer", "Social studies"},
	"mask": {"Maatschappijkunde", "Social sciences"},
	"maw":  {"Maatschappijwetenschappen", "Social sciences"},
	"men":  {"Mentoruur", "Mentor hour"},
	"ment": {"Mentoruur", "Mentor hour"},
	"mt":   {"Mentoruur", "Mentor hour"},
	"mu":   {"Muziek", "Music"},
	"na":   {"Natuurkunde", "Physics"},
	"nat":  {"Natuurkunde", "Physics"},
	"nask": {"Natuur- en scheikunde", "Physics and chemistry"},
	"ns":   {"Natuur- en scheikunde", "Physics and chemistry"},
	"ns1":  {"Natuur- en scheikunde 1", "Physics and chemistry 1"},
	"ns2":  {"Natuur- en scheikunde 2", "Physics and chemistry 2"},
	"ne":   {"Nederlands", "Dutch"},
	"netl": {"Nederlands", "Dutch"},
	"nlt":  {"Natuur, leven en technologie", "Nature, life and technology"},
	"o&o":  {"Onderzoek en ontwerpen", "Research and design"},
	"oo":   {"Onderzoek en ontwerpen", "Research and design"},
	"pws":  {"Profielwerkstuk", "Final research project"},
	"re":   {"Rekenen", "Arithmetic"},
	"rek":  {"Rekenen", "Arithmetic"},
	"ru":   {"Russisch", "Russian"},
	"sk":   {"Scheikunde", "Chemistry"},
	"schk": {"Scheikunde", "Chemistry"},
	"sp":   {"Spaans", "Spanish"},
	"sptl": {"Spaans", "Spanish"},
	"stu":  {"Studie-uur", "Study hour"},
	"st":   {"Studie-uur", "Study hour"},
	"te":   {"Tekenen", "Drawing"},
	"tk":   {"Tekenen", "Drawing"},
	"tn":   {"Techniek", "Technology"},
	"tech": {"Techniek", "Technology"},
	"tu":   {"Turks", "Turkish"},
	"vz":   {"Verzorging", "Health and care"},
	"wi":   {"Wiskunde", "Mathematics"},
	"wis":  {"Wiskunde", "Mathematics"},
	"wa":   {"Wiskunde A", "Mathematics A"},
	"wisa": {"Wiskunde A", "Mathematics A"},
	"wb":   {"Wiskunde B", "Mathematics B"},
	"wisb": {"Wiskunde B", "Mathematics B"},
	"wc":   {"Wiskunde C", "Mathematics C"},
	"wisc": {"Wiskunde C", "Mathematics C"},
	"wd":   {"Wiskunde D", "Mathematics D"},
	"wisd": {"Wiskunde D", "Mathematics D"},
}

var trailingDigits = regexp.MustCompile(`[0-9]+$`)

// Resolve finds the readable name for a code: the user's override first, then
// the defaults (also trying the code without a trailing level digit, so "wi2"
// reads as "Wiskunde"), and finally the code itself.
func Resolve(code, lang string, overrides map[string]string) (name string, known bool) {
	key := strings.ToLower(strings.TrimSpace(code))
	if n, ok := overrides[key]; ok {
		return n, true
	}
	n, ok := Defaults[key]
	if !ok {
		n, ok = Defaults[trailingDigits.ReplaceAllString(key, "")]
	}
	if !ok {
		return code, false
	}
	if lang == "en" {
		return n.EN, true
	}
	return n.NL, true
}

package metadata_test

import (
	"errors"
	"testing"

	"github.com/aydam2728/bookplate/internal/metadata"
)

// recordedBnF is what catalogue.bnf.fr returns for 9782021563313, trimmed to
// the elements this project reads. The shapes are the real ones: the title
// packs a subtitle and the author, the creator carries life dates and a role,
// and the publisher carries its city.
const recordedBnF = `<?xml version="1.0" encoding="UTF-8"?>
<srw:searchRetrieveResponse xmlns:srw="http://www.loc.gov/zing/srw/">
  <srw:numberOfRecords>1</srw:numberOfRecords>
  <srw:records><srw:record><srw:recordData>
    <oai_dc:dc xmlns:oai_dc="http://www.openarchives.org/OAI/2.0/oai_dc/"
               xmlns:dc="http://purl.org/dc/elements/1.1/">
      <dc:identifier>http://catalogue.bnf.fr/ark:/12148/cb47524066m</dc:identifier>
      <dc:title>Le mystère de la maison aux trois ormes : roman / Valentin Musso</dc:title>
      <dc:creator>Musso, Valentin (1977-....). Auteur du texte</dc:creator>
      <dc:publisher>Éditions du Seuil (Paris)</dc:publisher>
      <dc:date>2024</dc:date>
      <dc:identifier>ISBN 9782021563313</dc:identifier>
      <dc:format>1 vol. (364 p.) ; 22 cm</dc:format>
      <dc:language>fre</dc:language>
    </oai_dc:dc>
  </srw:recordData></srw:record></srw:records>
</srw:searchRetrieveResponse>`

// recordedBnFFuzzy is what the same index returns for an ISBN it does not
// have: unrelated books, confidently. This is the trap the provider guards
// against.
const recordedBnFFuzzy = `<?xml version="1.0" encoding="UTF-8"?>
<srw:searchRetrieveResponse xmlns:srw="http://www.loc.gov/zing/srw/">
  <srw:numberOfRecords>2</srw:numberOfRecords>
  <srw:records>
    <srw:record><srw:recordData>
      <oai_dc:dc xmlns:dc="http://purl.org/dc/elements/1.1/">
        <dc:title>Judith Rothchild : Lumière noire</dc:title>
        <dc:identifier>ISBN 0000000000</dc:identifier>
      </oai_dc:dc>
    </srw:recordData></srw:record>
    <srw:record><srw:recordData>
      <oai_dc:dc xmlns:dc="http://purl.org/dc/elements/1.1/">
        <dc:title>Dutch ships in tropical waters / Robert Parthesius</dc:title>
        <dc:identifier>ISBN 9789053565179</dc:identifier>
      </oai_dc:dc>
    </srw:recordData></srw:record>
  </srw:records>
</srw:searchRetrieveResponse>`

func TestBnF_ByISBN_ReadsARecord(t *testing.T) {
	s := &stub{body: recordedBnF}
	got, err := metadata.NewBnF(s).ByISBN(t.Context(), "9782021563313")
	if err != nil {
		t.Fatalf("ByISBN: %v", err)
	}

	if got.Title != "Le mystère de la maison aux trois ormes" {
		t.Errorf("Title = %q: the subtitle and the author must come off", got.Title)
	}
	if got.Subtitle != "roman" {
		t.Errorf("Subtitle = %q, want %q", got.Subtitle, "roman")
	}
	if len(got.Authors) != 1 || got.Authors[0] != "Valentin Musso" {
		t.Errorf("Authors = %v: a catalogue files a name as \"Musso, Valentin\", "+
			"nobody writes it that way on a shelf", got.Authors)
	}
	if got.Publisher != "Éditions du Seuil" {
		t.Errorf("Publisher = %q: the city must come off", got.Publisher)
	}
	if got.Published != "2024" || got.PageCount != 364 {
		t.Errorf("Published = %q, PageCount = %d", got.Published, got.PageCount)
	}
	if got.Provider != "bnf" {
		t.Errorf("Provider = %q", got.Provider)
	}
}

func TestBnF_ByISBN_RefusesARecordThatIsNotTheBookAskedFor(t *testing.T) {
	// bib.fuzzyISBN is exactly as fuzzy as its name says. Trusting it would
	// file a Dutch shipping history under a French novel's barcode.
	s := &stub{body: recordedBnFFuzzy}
	_, err := metadata.NewBnF(s).ByISBN(t.Context(), "9780000000002")
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("an unrelated record was accepted, err = %v", err)
	}
}

func TestBnF_ByISBN_MatchesAHyphenatedIdentifier(t *testing.T) {
	hyphenated := `<srw:searchRetrieveResponse xmlns:srw="http://www.loc.gov/zing/srw/">
	  <srw:records><srw:record><srw:recordData>
	    <oai_dc:dc xmlns:dc="http://purl.org/dc/elements/1.1/">
	      <dc:title>A book</dc:title>
	      <dc:identifier>ISBN 978-2-02-156331-3</dc:identifier>
	    </oai_dc:dc>
	  </srw:recordData></srw:record></srw:records>
	</srw:searchRetrieveResponse>`

	got, err := metadata.NewBnF(&stub{body: hyphenated}).ByISBN(t.Context(), "9782021563313")
	if err != nil {
		t.Fatalf("a hyphenated identifier was not recognised: %v", err)
	}
	if got.Title != "A book" {
		t.Errorf("Title = %q", got.Title)
	}
}

func TestBnF_ByISBN_WhenThereAreNoRecords_ReportsNotFound(t *testing.T) {
	empty := `<srw:searchRetrieveResponse xmlns:srw="http://www.loc.gov/zing/srw/">
	  <srw:numberOfRecords>0</srw:numberOfRecords></srw:searchRetrieveResponse>`
	_, err := metadata.NewBnF(&stub{body: empty}).ByISBN(t.Context(), "9782021563313")
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestBnF_ByISBN_WhenTheAnswerIsNotXML_Fails(t *testing.T) {
	_, err := metadata.NewBnF(&stub{body: "<html>maintenance</html>"}).
		ByISBN(t.Context(), "9782021563313")
	if err == nil {
		t.Error("an HTML page was accepted as a catalogue record")
	}
}

func TestChain_WhenOpenLibraryDoesNotKnow_TheBnFAnswers(t *testing.T) {
	// The case that prompted this provider: a Seuil 2024 that Open Library
	// has never heard of and that Google cannot find by its ISBN.
	chain := metadata.Chain{
		metadata.NewOpenLibrary(&stub{body: `{}`}),
		metadata.NewBnF(&stub{body: recordedBnF}),
	}

	got, err := chain.ByISBN(t.Context(), "9782021563313")
	if err != nil {
		t.Fatalf("the chain gave up: %v", err)
	}
	if got.Provider != "bnf" || got.Title != "Le mystère de la maison aux trois ormes" {
		t.Errorf("got %+v", got)
	}
}

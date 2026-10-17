package metadata_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/aydam2728/bookplate/internal/metadata"
)

// recordedInventaire is what inventaire.io returns for 9782021563313 with the
// relatives parameter, trimmed to what this project reads. The shapes are the
// real ones: everything is a Wikidata property, the edition labels itself
// under "fromclaims" while the related entities use language codes, and the
// cover is a path rather than a URL.
const recordedInventaire = `{
  "entities": {
    "inv:6b8a20d0": {
      "uri": "inv:6b8a20d0", "type": "edition",
      "labels": {"fromclaims": "Le mystère de la maison aux trois ormes"},
      "image": {"url": "/img/entities/9c2e23b5"},
      "claims": {
        "wdt:P1476": ["Le mystère de la maison aux trois ormes"],
        "wdt:P577": ["2024"],
        "wdt:P212": ["978-2-02-156331-3"],
        "wdt:P1104": [364],
        "wdt:P123": ["wd:Q2635075"],
        "wdt:P629": ["inv:eb245273"]
      }
    },
    "inv:eb245273": {
      "uri": "inv:eb245273", "type": "work",
      "labels": {"fr": "Le mystère de la maison aux trois ormes"},
      "claims": {"wdt:P50": ["wd:Q3552394"]}
    },
    "wd:Q2635075": {
      "uri": "wd:Q2635075", "type": "publisher",
      "labels": {"fr": "Éditions du Seuil", "zh": "門檻出版社"}, "claims": {}
    },
    "wd:Q3552394": {
      "uri": "wd:Q3552394", "type": "human",
      "labels": {"fr": "Valentin Musso", "bg": "Валантен Мюсо"}, "claims": {}
    }
  }
}`

func TestInventaire_ByISBN_ReadsARecord(t *testing.T) {
	got, err := metadata.NewInventaire(&stub{body: recordedInventaire}).
		ByISBN(t.Context(), "9782021563313")
	if err != nil {
		t.Fatalf("ByISBN: %v", err)
	}

	if got.Title != "Le mystère de la maison aux trois ormes" {
		t.Errorf("Title = %q", got.Title)
	}
	if len(got.Authors) != 1 || got.Authors[0] != "Valentin Musso" {
		t.Errorf("Authors = %v: the author lives on the work, two hops from "+
			"the edition", got.Authors)
	}
	if got.Publisher != "Éditions du Seuil" {
		t.Errorf("Publisher = %q: French is preferred over the other twenty "+
			"labels", got.Publisher)
	}
	if got.Published != "2024" || got.PageCount != 364 {
		t.Errorf("Published = %q, PageCount = %d", got.Published, got.PageCount)
	}
	if got.Provider != "inventaire" {
		t.Errorf("Provider = %q", got.Provider)
	}
}

func TestInventaire_ByISBN_MakesTheCoverURLAbsolute(t *testing.T) {
	got, err := metadata.NewInventaire(&stub{body: recordedInventaire}).
		ByISBN(t.Context(), "9782021563313")
	if err != nil {
		t.Fatalf("ByISBN: %v", err)
	}
	want := "https://" + metadata.InventaireHost + "/img/entities/9c2e23b5"
	if got.CoverURL != want {
		t.Errorf("CoverURL = %q, want %q: the API returns a path, and the "+
			"outbound client needs a full https URL", got.CoverURL, want)
	}
}

func TestInventaire_ByISBN_RefusesAnEditionWithADifferentISBN(t *testing.T) {
	wrong := strings.Replace(recordedInventaire, "978-2-02-156331-3", "978-0-14-032872-1", 1)
	_, err := metadata.NewInventaire(&stub{body: wrong}).ByISBN(t.Context(), "9782021563313")
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("a record carrying another ISBN was accepted, err = %v", err)
	}
}

func TestInventaire_ByISBN_AcceptsAnEditionThatStoresNoISBN13(t *testing.T) {
	// The request names an exact URI, so resolving at all is already
	// evidence. Refusing here would drop records that only carry an ISBN-10.
	none := strings.Replace(recordedInventaire, `"wdt:P212": ["978-2-02-156331-3"],`, "", 1)
	got, err := metadata.NewInventaire(&stub{body: none}).ByISBN(t.Context(), "9782021563313")
	if err != nil {
		t.Fatalf("a record without an ISBN-13 claim was refused: %v", err)
	}
	if got.Title == "" {
		t.Error("the record came back empty")
	}
}

func TestInventaire_ByISBN_WhenThereIsNoEdition_ReportsNotFound(t *testing.T) {
	for _, body := range []string{`{"entities":{}}`, `{}`} {
		_, err := metadata.NewInventaire(&stub{body: body}).ByISBN(t.Context(), "9782021563313")
		if !errors.Is(err, metadata.ErrNotFound) {
			t.Errorf("body %q gave err = %v, want ErrNotFound", body, err)
		}
	}
}

func TestInventaire_ByISBN_WhenTheAnswerIsNotJSON_Fails(t *testing.T) {
	_, err := metadata.NewInventaire(&stub{body: "<html>down</html>"}).
		ByISBN(t.Context(), "9782021563313")
	if err == nil {
		t.Error("an HTML page was accepted as a record")
	}
}

func TestChain_InventaireAnswersBeforeTheBnF(t *testing.T) {
	chain := metadata.Chain{
		metadata.NewOpenLibrary(&stub{body: `{}`}),
		metadata.NewInventaire(&stub{body: recordedInventaire}),
		metadata.NewBnF(&stub{body: recordedBnF}),
	}

	got, err := chain.ByISBN(t.Context(), "9782021563313")
	if err != nil {
		t.Fatalf("the chain gave up: %v", err)
	}
	if got.Provider != "inventaire" {
		t.Errorf("Provider = %q, want inventaire: it comes second and carries "+
			"a cover the national library does not", got.Provider)
	}
}

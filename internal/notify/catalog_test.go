package notify

import "testing"

// Every catalog event must provide a template for all 7 supported UI languages, so emails
// are localized to the recipient (see notifier.go's user.Language selection).
func TestCatalogAllLanguages(t *testing.T) {
	langs := []string{"en", "de", "uk", "fr", "es", "ru", "sr"}
	for key, ev := range Catalog {
		for _, lang := range langs {
			tpl, ok := ev.Templates[lang]
			if !ok {
				t.Errorf("event %q missing %q template", key, lang)
				continue
			}
			if tpl.Subject == "" || tpl.HTML == "" || tpl.Text == "" {
				t.Errorf("event %q/%s has an empty Subject/HTML/Text", key, lang)
			}
		}
	}
}

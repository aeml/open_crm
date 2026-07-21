package deals

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const quotePDFLinesPerPage = 43

var quoteFilenamePattern = regexp.MustCompile(`[^a-z0-9]+`)

var quoteWinAnsi = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87,
	'ˆ': 0x88, '‰': 0x89, 'Š': 0x8a, '‹': 0x8b, 'Œ': 0x8c, 'Ž': 0x8e, '‘': 0x91,
	'’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97, '˜': 0x98,
	'™': 0x99, 'š': 0x9a, '›': 0x9b, 'œ': 0x9c, 'ž': 0x9e, 'Ÿ': 0x9f,
}

type QuotePDFInput struct {
	OrganizationName string
	GeneratedByName  string
	GeneratedAt      time.Time
	QuoteNumber      string
	RecipientName    string
	RecipientEmail   string
	ValidUntil       string
	Terms            string
	Filename         string
	FXDisclosure     *QuoteFXDisclosure
}

type QuotePDFFile struct {
	Filename      string
	Content       []byte
	ContentSHA256 string
}

func BuildQuotePDF(detail Detail, input QuotePDFInput) QuotePDFFile {
	if input.GeneratedAt.IsZero() {
		input.GeneratedAt = time.Now().UTC()
	}
	organizationName := strings.TrimSpace(input.OrganizationName)
	if organizationName == "" {
		organizationName = "Open CRM"
	}
	generatedBy := strings.TrimSpace(input.GeneratedByName)
	if generatedBy == "" {
		generatedBy = "Open CRM"
	}

	lines := buildQuotePDFLines(detail, organizationName, generatedBy, input)
	filename := strings.TrimSpace(input.Filename)
	if filename == "" {
		filename = fmt.Sprintf("quote-%s.pdf", quoteFilename(detail.Summary.Name))
	}
	return QuotePDFFile{
		Filename: filename,
		Content:  renderTextPDF(lines),
	}
}

func buildQuotePDFLines(detail Detail, organizationName, generatedBy string, input QuotePDFInput) []string {
	deal := detail.Summary
	currency := normalizeQuoteCurrency(detail.Totals.Currency)
	if currency == "" {
		currency = normalizeQuoteCurrency(deal.ValueCurrency)
	}
	if currency == "" {
		currency = "USD"
	}

	lines := []string{
		organizationName,
		"Quote / Proposal",
		fmt.Sprintf("Generated: %s", input.GeneratedAt.UTC().Format("2006-01-02")),
		fmt.Sprintf("Prepared by: %s", generatedBy),
	}
	if strings.TrimSpace(input.QuoteNumber) != "" {
		lines = append(lines,
			fmt.Sprintf("Quote number: %s", strings.TrimSpace(input.QuoteNumber)),
			fmt.Sprintf("Valid until: %s", emptyQuoteValue(input.ValidUntil, "Not set")),
			fmt.Sprintf("Recipient: %s <%s>", emptyQuoteValue(input.RecipientName, "Not set"), emptyQuoteValue(input.RecipientEmail, "not-set")),
		)
	}
	lines = append(lines,
		"",
		fmt.Sprintf("Deal: %s", emptyQuoteValue(deal.Name, "Untitled deal")),
		fmt.Sprintf("Stage: %s", emptyQuoteValue(deal.StageName, "Not set")),
		fmt.Sprintf("Status: %s", emptyQuoteValue(deal.Status, "Not set")),
		fmt.Sprintf("Company: %s", emptyQuoteValue(deal.CompanyName, "No company linked")),
		fmt.Sprintf("Primary contact: %s", emptyQuoteValue(deal.PrimaryContactName, "No primary contact")),
		fmt.Sprintf("Expected close: %s", emptyQuoteValue(deal.ExpectedCloseDate, "Not set")),
		"",
		"Line items",
	)

	if len(detail.LineItems) == 0 {
		lines = append(lines, "No saved line items yet.")
		lines = append(lines, fmt.Sprintf("Deal value: %s", quoteMoney(deal.ValueAmount, currency)))
	} else {
		for _, item := range detail.LineItems {
			itemCurrency := normalizeQuoteCurrency(item.Currency)
			if itemCurrency == "" {
				itemCurrency = currency
			}
			line := fmt.Sprintf(
				"%d. %s%s - %s %s x %s, discount %s, tax %s%%, total %s",
				item.Position,
				emptyQuoteValue(item.Name, "Line item"),
				quoteSKU(item.SKU),
				emptyQuoteValue(item.Quantity, "0"),
				emptyQuoteValue(item.UnitName, "unit"),
				quoteMoney(item.UnitPrice, itemCurrency),
				quoteMoney(item.DiscountAmount, itemCurrency),
				emptyQuoteValue(item.TaxRate, "0"),
				quoteMoney(item.Total, itemCurrency),
			)
			lines = appendWrappedQuoteLine(lines, line, "   ")
		}
		lines = append(lines,
			"",
			"Totals",
			fmt.Sprintf("Subtotal: %s", quoteMoney(detail.Totals.Subtotal, currency)),
			fmt.Sprintf("Discount: %s", quoteMoney(detail.Totals.DiscountTotal, currency)),
			fmt.Sprintf("Tax: %s", quoteMoney(detail.Totals.TaxTotal, currency)),
			fmt.Sprintf("Total: %s", quoteMoney(detail.Totals.Total, currency)),
		)
	}

	if strings.TrimSpace(input.QuoteNumber) != "" && input.FXDisclosure != nil {
		disclosure := input.FXDisclosure
		lines = append(lines, "", "Currency disclosure")
		if currency == disclosure.BaseCurrency {
			lines = append(lines,
				fmt.Sprintf("Quote currency matches workspace base currency %s; no conversion was applied.", currency),
				fmt.Sprintf("Base-currency total: %s", quoteMoney(disclosure.TotalInBase, disclosure.BaseCurrency)),
			)
		} else {
			lines = append(lines,
				fmt.Sprintf("Workspace base currency: %s", disclosure.BaseCurrency),
				fmt.Sprintf("Rate: 1 %s = %s %s", currency, disclosure.RateToBase, disclosure.BaseCurrency),
			)
			lines = appendWrappedQuoteLine(lines,
				fmt.Sprintf("Effective: %s | Source: %s", disclosure.EffectiveDate, disclosure.Source), "   ")
			lines = append(lines,
				fmt.Sprintf("Reporting equivalent: %s", quoteMoney(disclosure.TotalInBase, disclosure.BaseCurrency)),
			)
			lines = appendWrappedQuoteLine(lines,
				fmt.Sprintf("Customer amount due remains %s; the %s equivalent is a reporting disclosure.", quoteMoney(detail.Totals.Total, currency), disclosure.BaseCurrency), "   ")
		}
	}

	if strings.TrimSpace(input.QuoteNumber) == "" {
		lines = append(lines,
			"",
			"Draft preview generated from current CRM deal data. Finalize a quote version before relying on its content.",
		)
	} else {
		lines = append(lines, "", "Terms")
		for _, paragraph := range strings.Split(strings.TrimSpace(input.Terms), "\n") {
			if strings.TrimSpace(paragraph) == "" {
				lines = append(lines, "")
				continue
			}
			lines = appendWrappedQuoteLine(lines, paragraph, "")
		}
		lines = append(lines,
			"",
			"Immutable finalized quote. SHA-256 verification is retained with the CRM record.",
		)
	}

	return lines
}

func renderTextPDF(lines []string) []byte {
	pages := chunkQuoteLines(lines)
	fontObjectID := 3 + len(pages)*2
	var body bytes.Buffer
	body.WriteString("%PDF-1.4\n")
	offsets := []int{0}

	writeObject := func(id int, content string) {
		offsets = append(offsets, body.Len())
		fmt.Fprintf(&body, "%d 0 obj\n%s\nendobj\n", id, content)
	}

	writeObject(1, "<< /Type /Catalog /Pages 2 0 R >>")
	kids := make([]string, 0, len(pages))
	for pageIndex := range pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", 3+pageIndex*2))
	}
	writeObject(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages)))

	for pageIndex, pageLines := range pages {
		pageObjectID := 3 + pageIndex*2
		contentObjectID := pageObjectID + 1
		stream := quotePageStream(pageLines)
		writeObject(pageObjectID, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", fontObjectID, contentObjectID))
		writeObject(contentObjectID, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}
	writeObject(fontObjectID, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")

	xrefOffset := body.Len()
	fmt.Fprintf(&body, "xref\n0 %d\n", len(offsets))
	body.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&body, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&body, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xrefOffset)
	return body.Bytes()
}

func quotePageStream(lines []string) string {
	var stream bytes.Buffer
	stream.WriteString("BT\n/F1 11 Tf\n54 738 Td\n14 TL\n")
	for _, line := range lines {
		fmt.Fprintf(&stream, "(%s) Tj\nT*\n", escapePDFText(line))
	}
	stream.WriteString("ET")
	return stream.String()
}

func chunkQuoteLines(lines []string) [][]string {
	if len(lines) == 0 {
		return [][]string{{"Quote / Proposal"}}
	}
	pages := make([][]string, 0, len(lines)/quotePDFLinesPerPage+1)
	for len(lines) > 0 {
		end := quotePDFLinesPerPage
		if len(lines) < end {
			end = len(lines)
		}
		pages = append(pages, lines[:end])
		lines = lines[end:]
	}
	return pages
}

func appendWrappedQuoteLine(lines []string, line, continuationPrefix string) []string {
	const maxLineLength = 92
	line = strings.TrimSpace(line)
	if len(line) <= maxLineLength {
		return append(lines, line)
	}

	current := ""
	for _, word := range strings.Fields(line) {
		candidate := strings.TrimSpace(current + " " + word)
		if len(candidate) > maxLineLength && current != "" {
			lines = append(lines, current)
			current = continuationPrefix + word
			continue
		}
		current = candidate
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func escapePDFText(value string) string {
	var builder strings.Builder
	for _, r := range value {
		encoded, ok := quoteWinAnsiByte(r)
		if !ok {
			encoded = '?'
		}
		switch encoded {
		case '\\', '(', ')':
			builder.WriteByte('\\')
			builder.WriteByte(encoded)
		case '\n', '\r', '\t':
			builder.WriteByte(' ')
		default:
			if encoded < 32 {
				builder.WriteByte('?')
			} else {
				builder.WriteByte(encoded)
			}
		}
	}
	return builder.String()
}

func quoteWinAnsiByte(r rune) (byte, bool) {
	if r <= 0x7f || (r >= 0xa0 && r <= 0xff) {
		// #nosec G115 -- this branch explicitly bounds the rune to the complete one-byte range before conversion.
		return byte(r), true
	}
	encoded, ok := quoteWinAnsi[r]
	return encoded, ok
}

func quoteMoney(amount, currency string) string {
	currency = normalizeQuoteCurrency(currency)
	if currency == "" {
		currency = "USD"
	}
	amount = strings.TrimSpace(amount)
	if amount == "" {
		amount = "0"
	}
	parsed, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return currency + " " + amount
	}
	return fmt.Sprintf("%s %.2f", currency, parsed)
}

func normalizeQuoteCurrency(currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if lineItemCurrencyPattern.MatchString(currency) {
		return currency
	}
	return ""
}

func quoteSKU(sku string) string {
	sku = strings.TrimSpace(sku)
	if sku == "" {
		return ""
	}
	return fmt.Sprintf(" (%s)", sku)
}

func emptyQuoteValue(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func quoteFilename(name string) string {
	filename := strings.ToLower(strings.TrimSpace(name))
	filename = quoteFilenamePattern.ReplaceAllString(filename, "-")
	filename = strings.Trim(filename, "-")
	if filename == "" {
		return "deal"
	}
	if len(filename) > 80 {
		filename = strings.Trim(filename[:80], "-")
	}
	if filename == "" {
		return "deal"
	}
	return filename
}

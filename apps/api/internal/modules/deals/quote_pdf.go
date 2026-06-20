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

type QuotePDFInput struct {
	OrganizationName string
	GeneratedByName  string
	GeneratedAt      time.Time
}

type QuotePDFFile struct {
	Filename string
	Content  []byte
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

	lines := buildQuotePDFLines(detail, organizationName, generatedBy, input.GeneratedAt)
	return QuotePDFFile{
		Filename: fmt.Sprintf("quote-%s.pdf", quoteFilename(detail.Summary.Name)),
		Content:  renderTextPDF(lines),
	}
}

func buildQuotePDFLines(detail Detail, organizationName, generatedBy string, generatedAt time.Time) []string {
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
		fmt.Sprintf("Generated: %s", generatedAt.UTC().Format("2006-01-02")),
		fmt.Sprintf("Prepared by: %s", generatedBy),
		"",
		fmt.Sprintf("Deal: %s", emptyQuoteValue(deal.Name, "Untitled deal")),
		fmt.Sprintf("Stage: %s", emptyQuoteValue(deal.StageName, "Not set")),
		fmt.Sprintf("Status: %s", emptyQuoteValue(deal.Status, "Not set")),
		fmt.Sprintf("Company: %s", emptyQuoteValue(deal.CompanyName, "No company linked")),
		fmt.Sprintf("Primary contact: %s", emptyQuoteValue(deal.PrimaryContactName, "No primary contact")),
		fmt.Sprintf("Expected close: %s", emptyQuoteValue(deal.ExpectedCloseDate, "Not set")),
		"",
		"Line items",
	}

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

	lines = append(lines,
		"",
		"Quote generated from current CRM deal data. Signature workflow, approvals, and terms remain future slices.",
	)

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
	writeObject(fontObjectID, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

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
		switch r {
		case '\\', '(', ')':
			builder.WriteByte('\\')
			builder.WriteRune(r)
		case '\n', '\r', '\t':
			builder.WriteByte(' ')
		default:
			if r < 32 || r > 126 {
				builder.WriteByte('?')
			} else {
				builder.WriteRune(r)
			}
		}
	}
	return builder.String()
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

# Native XLS formatting

`xls_format.json` preserves the formatting of `ТехОбъекты_diag.xls` supplied
with the project (SHA-256
`0be1991b6fdc46ddaab8393e95dfb2f9e96b96643f814b63d0e0d7cf3819e6f4`).

The base64 fields contain BIFF8 FONT, XF and FORMAT record payloads, without
record headers. They retain native indices, including Excel's unused font 4.
No object values, formulas, scripts or external workbook links are included.

The header uses 100 columns and six merged ranges: A1:B2, C1:T2, U1:U2,
V1:V2, W1:W2 and the blank X1:CV2 band. Rows 1–4 are 255, 255, 600 and
225 twips high; the default column width is 8. Header XF indices are 66,
66, 63 and 64; object rows use 65. This preserves centered headings,
wrapped Russian captions, borders, the gray field-code row and the smaller
Arial 8 font on that row. Other rows use Arial 10.

Merged cells retain their original hidden values in row 2 because those
values identify property groups for SCADA import. Formatting never clears
or relocates the table's import fields.

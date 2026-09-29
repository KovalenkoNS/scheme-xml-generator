These are the user-provided PLC850 SKZ mapping examples, copied unchanged for portable parser regression tests:

- `ai.xlsx`: `PS_GDS_Перекладка_AI_Channel_v0.xlsx`, 976 assignments forming 488 Xin/Xs pairs across 32 modules and 4 rack groups.
- `do.xlsx`: `PS_GDS_Перекладка_DO_Channel_v0.xlsx`, 144 physical assignments across 8 modules and 2 rack groups. Each of 36 source output variables drives four distinct destinations.
- `do_native.xlsx`: byte-exact copy of `../XML dev/DO генерация (1).xlsx`, 159128 bytes. SHA256: `B734A80FB656986F1079AEAC48304205E5809D2E321B929FEA86740CC796A4C3`. The prepared `SCS DO` RHS is authoritative, including Loop-derived `_XYC` names that differ from `Марка`; the raw IO `Tag No + _DDVH` rule is not applied to this format. `Лист1` starts at row 1 and contains 408 B03 assignments (16 modules / 2 groups; 344 `простой`, 64 `DO-1`); `Лист2` starts at row 2 and contains 1284 B01 assignments (44 modules / 8 groups; 400 `простой`, 884 `DO-1`). Combined: 1692 assignments, 60 modules and 10 groups. Row templates are retained in `Channel.Template`; differing per-row module metadata is not asserted as a uniform module property.

The files contain neither numeric module IDs nor free reserve channels. AI object names ending in `_reserve` represent working redundant signals. Missing physical positions remain absent from the parsed plan.

Raw mixed DI/DO source (separate from the prepared AI/DO maps above):

- `di.xlsx`: byte-exact copy of `../XML dev/PS_IO_LIST_SCS_v4_new.xlsx` supplied by the owner. SHA256: `C9C10D1E86936AC5B5BE17B76E2BDEFEDF71F97321938D1373CA179884879339`.
- Sheet `PS_IO_LIST_SCS_v4` contains 619 DI rows. Main_module maps to DDR.C1 and Redundant_module to the same DDR.C2, as confirmed by the owner. One SPARE row has no receiver, leaving 1236 mapped channels in 50 modules / 8 groups / 3 PLCs.
- Tag No supplies names without changing LZS to LZSA. The SPARE row retains the module and physical ST inputs; named reserved DDR signals remain connected. Numeric physical IDs must be entered separately.
- The same unchanged fixture also contains 487 DO rows (`DOR-P`, `DOR-VFC`, `DOR (SCS3)`). Tag No supplies the BOOL name: replace hyphens with underscores, add a leading underscore if absent, then append `_DDVH`. Loop is not the source of this name.
- Each DO row has four explicit placements: Main_module, Main_module2, Redundant_module and Redundant_module2, all using the same Channel and source. They produce 1948 assignments in 72 modules / 12 groups / 3 PLCs. The parser also accepts fewer placements when optional fields are empty; Main_module is mandatory. The sample has no DO SPARE/blank Tag No; such unknown reserve names are rejected rather than invented.
- Combined DI/DO output: 3184 mapped channels in 122 modules and 20 groups. Other I/O types in the raw workbook are ignored with a warning. The historical fixture name `di.xlsx` and its bytes are preserved.

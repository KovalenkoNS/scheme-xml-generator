These are the user-provided PLC850 SKZ mapping examples, copied unchanged for portable parser regression tests:

- `ai.xlsx`: `PS_GDS_Перекладка_AI_Channel_v0.xlsx`, 976 assignments forming 488 Xin/Xs pairs across 32 modules and 4 rack groups.
- `do.xlsx`: `PS_GDS_Перекладка_DO_Channel_v0.xlsx`, 144 physical assignments across 8 modules and 2 rack groups. Each of 36 source output variables drives four distinct destinations.

The files contain neither numeric module IDs nor free reserve channels. AI object names ending in `_reserve` represent working redundant signals. Missing physical positions remain absent from the parsed plan.

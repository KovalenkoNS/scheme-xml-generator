# Native PLC diagnostic profile

`plc_diagnostic_profile.xml.gz` is a compressed, embedded subset of the supplied
SCADA export `output/3000_D_SC_B01.xml`. It contains:

- the three original rack/chassis pictures, with their original hex data;
- the native color styles and ten named mnemonic-symbol dependencies;
- front/rear pagination and acknowledgement-button primitive appearances.

The original receptors and all object/channel bindings were removed. At runtime
the generator reconstructs those exclusively from the parsed IO inventory and
the newly allocated IDs. The export's invalid mixed-encoding layer names are not
part of this asset; generated layer names use valid UTF-8.

This asset is included in the executable and needs no files from `output` at
runtime. `PAGEMSINFO` references existing SCADA mnemonic symbols, not embedded
symbol definitions. The cabinet pages also retain the named external template
`Служебные шаблоны\Шаблон диагностики контроллеров_PS` and parent group
`Диагностика`, as in the native export.

"""Create small, explicit AI/AO/DI/DO source tables for browser acceptance.
Only test-owned .artifacts directories are written; installed user files are untouched.
"""
from pathlib import Path
from xml.sax.saxutils import escape
import sys
import zipfile


def workbook(target, rows):
    """Write inline-string XLSX cells in source order, without formulas or macros."""
    worksheet = '<worksheet><sheetData>'
    for number, row in enumerate(rows, 1):
        worksheet += f'<row r="{number}">'
        for column, value in enumerate(row):
            worksheet += f'<c r="{chr(65 + column)}{number}" t="inlineStr"><is><t>{escape(str(value))}</t></is></c>'
        worksheet += '</row>'
    worksheet += '</sheetData></worksheet>'
    with zipfile.ZipFile(target, 'w', zipfile.ZIP_DEFLATED) as archive:
        archive.writestr('_rels/.rels', '<Relationships><Relationship Id="book" Type="urn:test/officeDocument" Target="xl/workbook.xml"/></Relationships>')
        archive.writestr('xl/workbook.xml', '<workbook xmlns:r="urn:test"><sheets><sheet name="IO" sheetId="1" r:id="s1"/></sheets></workbook>')
        archive.writestr('xl/_rels/workbook.xml.rels', '<Relationships><Relationship Id="s1" Type="urn:test/worksheet" Target="worksheets/sheet1.xml"/></Relationships>')
        archive.writestr('xl/worksheets/sheet1.xml', worksheet)


def main(target):
    """Build four IO inventory fixtures and one AO text map, each containing two PLCs."""
    root = Path(__file__).resolve().parents[2]
    target = target.resolve()
    if not target.is_relative_to(root / '.artifacts'):
        raise ValueError('Matrix fixtures must stay inside .artifacts')
    target.mkdir(parents=True, exist_ok=True)
    header = ['FCS', 'Control cabinet', 'ModuleType', 'Main module', 'Redundand module', 'Channel', 'Loop', 'Tag No', 'SCADA Tag']
    for kind, module in [('AI', 'AI16H'), ('AO', 'AOC4H'), ('DI', 'DI32'), ('DO', 'DO32P')]:
        rows = [header]
        for plc in ['A', 'B']:
            rows.append([f'VERIFY_SC_{plc}', f'VERIFY_SC_{plc}', module, 'A11-02', '-', 0, f'100-TT-{plc}', f'100-TT-{plc}', f'_VERIFY_{kind}_{plc}'])
        workbook(target / f'{kind}.xlsx', rows)
    ao = ['FCS\tMashalling_cabinet\tModule\tChannel\tDCS AO\tMain_module\tRedundant_module\tI/O Type\tТип объекта']
    for plc in ['A', 'B']:
        ao.append(f'VERIFY_SC_{plc}\tCAB\tA11-02\t0\t_IO_QU*A11-02*_0.ValueDINT := REAL_TO_DINT(_VERIFY_AO_{plc}.OUT, 0.0, 100.0);\tA11-02\t\tAO\tAN_v1')
    (target / 'AO.txt').write_text('\n'.join(ao) + '\n', encoding='utf-8')
    raw_header = ['Tag No', 'SCS', 'I/O Type', 'Main_module', 'Redundant_module', 'Channel']
    for kind, io_type in [('DI', 'DIR-VFC'), ('DO', 'DOR-P')]:
        workbook(target / f'ST-{kind}.xlsx', [raw_header] + [[f'VERIFY-{kind}-{plc}', f'VERIFY_SC_{plc}', io_type, 'A11-02', '-', 0] for plc in ['A', 'B']])
    ai_header = ['Loop', 'SCS', 'Mashalling_cabinet', 'Module', 'Channel', 'SCS AI', 'Main_module', 'Redundant_module', 'I/O Type', 'Тип объекта', 'Шаблон', 'Марка']
    ai = [ai_header]
    for plc in ['A', 'B']:
        tag = f'_VERIFY_AI_{plc}'
        for statement in [f'{tag}.Xin := _IO_I*A11-02*_AI16H_0_VAL.Measurement;', f'{tag}.Xs := QUAL_STAT(_IO_I*A11-02*_AI16H_0_VAL.Quality);']:
            ai.append(['100-TT-1', f'VERIFY_SC_{plc}', 'CAB', 'A11-02', 0, statement, 'A11-02', '-', 'AI', 'AD3_v2', 'AD3_v2', tag])
    workbook(target / 'ST-AI.xlsx', ai)


if __name__ == '__main__':
    main(Path(sys.argv[1]))

"""Generate checked-in, typed MyBatis-Plus entities and mapper declarations.

Schema metadata is a migration input; it is never a client-controlled table registry.
Run from any directory after reviewing schema-metadata.json changes.
"""
import json
from pathlib import Path

root = Path(__file__).resolve().parents[1]
schema = json.loads((root / 'src/main/resources/schema-metadata.json').read_text())
java = root / 'src/main/java/com/kasinlo/erp/persistence'

def classname(table):
    return ''.join(part.title() for part in table.split('_'))

def java_type(field):
    return {'long': 'Long', 'bool': 'Boolean', 'decimal': 'BigDecimal',
            'time': 'LocalDate' if field.get('sqlType') == 'date' else 'OffsetDateTime',
            'string': 'String'}[field['type']]

entries = []
for table, fields in schema.items():
    name = classname(table)
    soft = all(key in fields for key in ('id', 'createdAt', 'updatedAt', 'deletedAt'))
    parent = 'SoftDeleteEntity' if soft else 'IdentifiedEntity' if 'id' in fields else None
    inherited = {'id', 'createdAt', 'updatedAt', 'deletedAt'} if soft else {'id'} if parent else set()
    lines = ['package com.kasinlo.erp.persistence.entity;', '',
             'import com.baomidou.mybatisplus.annotation.TableField;',
             'import com.baomidou.mybatisplus.annotation.TableName;',
             'import java.math.BigDecimal;', 'import java.time.LocalDate;',
             'import java.time.OffsetDateTime;', '',
             '/** Persistence mapping for ' + table + '. */',
             '@TableName("' + table + '")',
             'public class ' + name + (' extends ' + parent if parent else '') + ' {']
    for field, meta in fields.items():
        if field in inherited:
            continue
        column = meta['column']
        escaped = '\\"' + column + '\\"'
        lines += ['    @TableField("' + escaped + '")',
                  '    private ' + java_type(meta) + ' ' + field + ';', '']
    for field, meta in fields.items():
        if field in inherited:
            continue
        cap = field[0].upper() + field[1:]
        typ = java_type(meta)
        lines += ['    public ' + typ + ' get' + cap + '() { return ' + field + '; }',
                  '    public void set' + cap + '(' + typ + ' ' + field + ') { this.' + field + ' = ' + field + '; }', '']
    lines += ['}', '']
    entity = java / 'entity' / (name + '.java')
    entity.parent.mkdir(parents=True, exist_ok=True)
    entity.write_text('\n'.join(lines))
    mapper = java / 'mapper' / (name + 'Mapper.java')
    mapper.parent.mkdir(parents=True, exist_ok=True)
    mapper.write_text('package com.kasinlo.erp.persistence.mapper;\n\n'
                      'import com.baomidou.mybatisplus.core.mapper.BaseMapper;\n'
                      'import com.kasinlo.erp.persistence.entity.' + name + ';\n\n'
                      'public interface ' + name + 'Mapper extends BaseMapper<' + name + '> {}\n')
    entries.append('        Map.entry("' + table + '", new Mapping(' + name + '.class, ' + name + 'Mapper.class))')

(java / 'EntityMappings.java').write_text('''package com.kasinlo.erp.persistence;

import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import com.kasinlo.erp.persistence.entity.*;
import com.kasinlo.erp.persistence.mapper.*;
import java.util.Map;

/** Compile-time allowlist. ORM identifiers never come from HTTP input. */
public final class EntityMappings {
    public record Mapping(Class<?> entityType, Class<? extends BaseMapper<?>> mapperType) {}
    private static final Map<String, Mapping> TABLES = Map.ofEntries(
''' + ',\n'.join(entries) + '''
    );
    private EntityMappings() {}
    public static Mapping require(String table) {
        Mapping mapping = TABLES.get(table);
        if (mapping == null) throw new IllegalArgumentException("Unknown table");
        return mapping;
    }
}
''')
print('Generated', len(schema), 'entity/mapper pairs')

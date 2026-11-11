import React from 'react';

export interface Column<T> {
  header: string;
  accessorKey?: keyof T;
  cell?: (item: T) => React.ReactNode;
  align?: 'left' | 'center' | 'right';
  className?: string;
}

export interface TableProps<T> {
  columns: Column<T>[];
  data: T[];
  keyExtractor: (item: T, index: number) => string | number;
  emptyMessage?: string;
  className?: string;
}

export function Table<T>({
  columns,
  data,
  keyExtractor,
  emptyMessage = 'NO RECORDS FOUND',
  className = '',
}: TableProps<T>) {
  if (data.length === 0) {
    return (
      <div className="py-12 text-center font-mono text-xs text-muted border border-hairline">
        [ {emptyMessage} ]
      </div>
    );
  }

  return (
    <div className={`overflow-x-auto border border-hairline ${className}`}>
      <table className="w-full text-left text-xs border-collapse">
        <thead>
          <tr className="border-b border-hairline bg-cream">
            {columns.map((col, idx) => (
              <th
                key={idx}
                scope="col"
                className={`py-3 px-4 font-mono font-bold uppercase tracking-wider text-muted text-[11px] ${
                  col.align === 'right'
                    ? 'text-right'
                    : col.align === 'center'
                    ? 'text-center'
                    : 'text-left'
                } ${col.className || ''}`}
              >
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-hairline font-mono">
          {data.map((item, rowIdx) => (
            <tr
              key={keyExtractor(item, rowIdx)}
              className="hover:bg-[rgba(255,255,255,0.2)] transition-colors duration-300"
            >
              {columns.map((col, colIdx) => (
                <td
                  key={colIdx}
                  className={`py-3 px-4 align-middle text-jet ${
                    col.align === 'right'
                      ? 'text-right'
                      : col.align === 'center'
                      ? 'text-center'
                      : 'text-left'
                  } ${col.className || ''}`}
                >
                  {col.cell
                    ? col.cell(item)
                    : col.accessorKey
                    ? String(item[col.accessorKey] ?? '')
                    : null}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default Table;

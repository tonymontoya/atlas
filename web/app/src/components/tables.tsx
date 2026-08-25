import type { ReactNode } from "react";
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
} from "@carbon/react";
import { EmptyState } from "./ui";

export type Column<T> = {
  key: string;
  header: ReactNode;
  render: (row: T) => ReactNode;
};

// AtlasTable renders read-only tables on Carbon's Table primitives.
// Carbon's DataTable component is a client-side state machine for sort,
// selection, and filtering; Atlas lists are server-searched and
// server-paginated (ADR-0028 deviation, recorded here for reviewers).
export function AtlasTable<T>({
  title,
  description,
  columns,
  rows,
  rowKey,
  emptyLabel,
}: {
  title?: ReactNode;
  description?: ReactNode;
  columns: Column<T>[];
  rows: T[];
  rowKey: (row: T) => string;
  emptyLabel: string;
}) {
  return (
    <TableContainer
      title={title}
      description={description}
      className={title === undefined ? "atlas-table-untitled" : undefined}
    >
      {rows.length === 0 ? (
        <EmptyState label={emptyLabel} />
      ) : (
        <Table>
          <TableHead>
            <TableRow>
              {columns.map((column) => (
                <TableHeader key={column.key} id={column.key}>
                  {column.header}
                </TableHeader>
              ))}
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={rowKey(row)}>
                {columns.map((column) => (
                  <TableCell key={column.key}>{column.render(row)}</TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </TableContainer>
  );
}

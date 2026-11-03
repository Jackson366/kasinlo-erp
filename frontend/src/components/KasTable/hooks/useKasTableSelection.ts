import { shallowRef } from 'vue'
import type { KasTableHandle } from '../types'

export function useKasTableSelection<Row>() {
  const selection = shallowRef<Row[]>([])

  function onSelectionChange(rows: Row[]): void {
    selection.value = rows
  }

  function clearSelection(table?: KasTableHandle | null): void {
    table?.clearSelection()
    selection.value = []
  }

  return { selection, onSelectionChange, clearSelection }
}

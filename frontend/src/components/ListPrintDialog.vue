<script setup lang="ts" generic="T extends Record<string, any>">
/**
 * 列表打印弹窗（通用）。
 *
 * 为什么不复用单据打印（OrderSheet / StockSheet）：那里是**一张单据**，
 * 有模板、有合计、有大小写金额；列表打印要的是「把表格横向铺到 A4」，
 * 两者版式目标不同。共用只会让单据模板里长出一堆「如果是列表就……」的分支。
 *
 * 打印走 body.printing 这个开关（与单据打印同一套全局规则），
 * 全局样式里只保留 .list-sheet 可见 —— 所以这里不需要把弹窗内容搬到新窗口，
 * 也不用处理跨窗口的样式丢失。
 */
import { computed } from 'vue'

export interface ListPrintColumn<Row> {
  label: string
  /** 取字段名；与 value 二选一 */
  prop?: keyof Row & string
  /** 自定义取值（金额格式化、状态翻译等） */
  value?: (row: Row) => string
  width?: string
  align?: 'left' | 'center' | 'right'
}

const props = defineProps<{
  /** v-model 控制显隐 */
  modelValue: boolean
  title: string
  /** 副标题：筛选条件 / 条数 / 打印时间，由页面拼好传入 */
  subtitle?: string
  columns: ListPrintColumn<T>[]
  rows: T[]
}>()

const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v),
})

const now = computed(() => {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
})

function cell(row: T, col: ListPrintColumn<T>): string {
  if (col.value) return col.value(row)
  if (!col.prop) return ''
  const v = row[col.prop]
  return v === null || v === undefined || v === '' ? '-' : String(v)
}

function doPrint(): void {
  document.body.classList.add('printing')
  const cleanup = () => document.body.classList.remove('printing')
  window.addEventListener('afterprint', cleanup, { once: true })
  window.print()
  // 部分浏览器（旧版 Firefox）不触发 afterprint，兜底摘掉类，
  // 否则关掉打印框后整页还停留在「全部隐藏」的状态
  window.setTimeout(cleanup, 1500)
}
</script>

<template>
  <el-dialog v-model="visible" title="打印预览" width="1180px" top="4vh" destroy-on-close>
    <div class="print-preview">
      <div class="list-sheet">
        <div class="list-sheet__head">
          <h2>{{ title }}</h2>
          <p class="list-sheet__meta">
            <span v-if="subtitle">{{ subtitle }}</span>
            <span>共 {{ rows.length }} 条</span>
            <span>打印时间 {{ now }}</span>
          </p>
        </div>
        <table class="list-sheet__table">
          <thead>
            <tr>
              <th class="list-sheet__idx">#</th>
              <th
                v-for="c in columns"
                :key="c.label"
                :style="{ width: c.width, textAlign: c.align || 'left' }"
              >
                {{ c.label }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, i) in rows" :key="i">
              <td class="list-sheet__idx">{{ i + 1 }}</td>
              <td
                v-for="c in columns"
                :key="c.label"
                :style="{ textAlign: c.align || 'left' }"
              >
                {{ cell(row, c) }}
              </td>
            </tr>
            <tr v-if="rows.length === 0">
              <td :colspan="columns.length + 1" class="list-sheet__empty">没有可打印的数据</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
      <el-button type="primary" :disabled="rows.length === 0" @click="doPrint">打印</el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.print-preview {
  max-height: 68vh;
  overflow: auto;
  padding: 8px;
  background: var(--app-fill-lighter);
}

.list-sheet {
  width: 1123px; // A4 横向 @96dpi
  margin: 0 auto;
  padding: 28px 32px 32px;
  background: #fff;
  color: #1f2937;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.12);

  &__head {
    margin-bottom: 14px;
    text-align: center;

    h2 {
      margin: 0 0 6px;
      font-size: 20px;
      font-weight: 700;
      letter-spacing: 2px;
    }
  }

  &__meta {
    display: flex;
    justify-content: center;
    gap: 16px;
    margin: 0;
    font-size: 12px;
    color: #6b7280;
  }

  &__table {
    width: 100%;
    border-collapse: collapse;
    table-layout: auto;
    font-size: 12px;

    th,
    td {
      padding: 6px 8px;
      border: 1px solid #d1d5db;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 240px;
    }

    th {
      background: #f3f4f6;
      font-weight: 600;
      color: #111827;
    }

    tbody tr:nth-child(even) td {
      background: #fafafa;
    }
  }

  &__idx {
    width: 40px;
    text-align: center;
    color: #6b7280;
  }

  &__empty {
    padding: 24px;
    text-align: center;
    color: #9ca3af;
  }
}
</style>

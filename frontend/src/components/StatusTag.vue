<script setup lang="ts">
/**
 * 启用 / 停用 状态标签。
 *
 * menu/dept/role 三个系统页曾在各自的表格里写同一串三元表达式
 * （type 取 success/info、尺寸 effect 也一致）。状态列的配色口径必须全站一致，
 * 收敛成一个组件：以后调色或改尺寸只改一处，不会漏掉某一页。
 */
withDefaults(
  defineProps<{
    /** 1 = 启用，其余视为停用 */
    status: number
    /**
     * 停用文案。默认「停用」（部门/菜单用），角色页用「禁用」。
     * 两个词在现有页面里并存，统一属于产品文案决策，这里先原样保留。
     */
    disabledText?: string
  }>(),
  { disabledText: '停用' },
)
</script>

<template>
  <el-tag :type="status === 1 ? 'success' : 'info'" size="small" effect="light">
    {{ status === 1 ? '启用' : disabledText }}
  </el-tag>
</template>

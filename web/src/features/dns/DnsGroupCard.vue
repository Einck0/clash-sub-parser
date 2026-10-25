<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronDownIcon } from '@heroicons/vue/24/outline'
import { uiText } from '../../locales/redesign'
import {
  type DnsGroupDef,
  countGroupConfigured,
} from './dnsFields'
import DnsFieldControl from './DnsFieldControl.vue'

const props = withDefaults(
  defineProps<{
    group: DnsGroupDef
    draft: Record<string, unknown>
    fieldErrors?: Record<string, string>
    invalidJsonInputs?: Record<string, string>
    disabled?: boolean
    defaultOpen?: boolean
  }>(),
  {
    fieldErrors: () => ({}),
    invalidJsonInputs: () => ({}),
    disabled: false,
    defaultOpen: false,
  }
)

defineEmits<{
  (e: 'update-list', key: string, text: string): void
  (e: 'update-scalar', key: string, val: string): void
  (e: 'update-bool', key: string, val: boolean): void
  (e: 'update-json', key: string, text: string): void
  (e: 'remove-key', key: string): void
}>()

const isOpen = ref(props.defaultOpen)

function toggleOpen() {
  isOpen.value = !isOpen.value
}

const configuredCount = computed(() => {
  return countGroupConfigured(props.draft, props.group)
})

const hasErrorInGroup = computed(() => {
  return props.group.fields.some((f) => Boolean(props.fieldErrors[f.key]))
})
</script>

<template>
  <div
    class="border border-base-300 rounded-xl bg-base-200/50 shadow-xs transition-colors"
    :class="{ 'border-error/40': hasErrorInGroup }"
  >
    <!-- Accordion Trigger Button -->
    <button
      type="button"
      class="w-full flex items-center justify-between p-3 sm:p-4 text-left cursor-pointer hover:bg-base-200/80 transition-colors focus:outline-hidden focus-visible:ring-2 focus-visible:ring-primary/40 rounded-xl"
      :aria-expanded="isOpen"
      :aria-controls="`group-content-${group.id}`"
      :data-testid="`dns-group-toggle-${group.id}`"
      @click="toggleOpen"
    >
      <div class="min-w-0 flex-1 pr-3">
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-xs sm:text-sm font-bold text-base-content tracking-tight">
            {{ uiText(group.titleZh, group.titleEn) }}
          </span>

          <!-- Configured count badge -->
          <span
            v-if="configuredCount > 0"
            class="badge badge-sm badge-primary text-[10px] font-medium"
            :data-testid="`dns-group-count-${group.id}`"
          >
            {{ configuredCount }} {{ uiText('项已配置', 'configured') }}
          </span>
          <span
            v-else
            class="badge badge-sm badge-ghost opacity-60 text-[10px]"
            :data-testid="`dns-group-count-${group.id}`"
          >
            {{ uiText('未配置', 'None configured') }}
          </span>

          <!-- Error badge -->
          <span
            v-if="hasErrorInGroup"
            class="badge badge-sm badge-error text-[10px] font-semibold"
          >
            {{ uiText('格式错误', 'Format error') }}
          </span>
        </div>

        <p class="text-[11px] text-base-content/60 leading-tight mt-1 truncate">
          {{ uiText(group.descZh, group.descEn) }}
        </p>
      </div>

      <div class="shrink-0 flex items-center">
        <ChevronDownIcon
          class="w-4 h-4 text-base-content/60 transition-transform duration-200"
          :class="{ 'rotate-180': isOpen }"
          aria-hidden="true"
        />
      </div>
    </button>

    <!-- Disclosed Content (v-show ensures inputs & incomplete drafts remain mounted) -->
    <div
      v-show="isOpen"
      :id="`group-content-${group.id}`"
      class="p-3 sm:p-4 border-t border-base-300/60"
      :data-testid="`dns-group-content-${group.id}`"
    >
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3 sm:gap-4">
        <DnsFieldControl
          v-for="field in group.fields"
          :key="field.key"
          :field="field"
          :draft="draft"
          :field-error="fieldErrors[field.key]"
          :invalid-json-input="invalidJsonInputs[field.key]"
          :disabled="disabled"
          @update-list="(k, t) => $emit('update-list', k, t)"
          @update-scalar="(k, v) => $emit('update-scalar', k, v)"
          @update-bool="(k, b) => $emit('update-bool', k, b)"
          @update-json="(k, t) => $emit('update-json', k, t)"
          @remove-key="(k) => $emit('remove-key', k)"
        />
      </div>
    </div>
  </div>
</template>

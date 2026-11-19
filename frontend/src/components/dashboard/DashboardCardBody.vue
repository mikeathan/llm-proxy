<script setup lang="ts">
import { provide } from "vue"
import { IN_DASHBOARD_CARD } from "../../constants/dashboard"

// The content area of a card. Everything inside it is "in a card": a Panel there draws only its content, because the card around it is the
// panel. This is a component of its own, not part of the frame, because the frame draws its own Panel, which must not unwrap itself.
provide(IN_DASHBOARD_CARD, true)
defineSlots<{ default(): unknown }>()
</script>

<template><slot /></template>

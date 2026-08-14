<template>
  <div style="min-height:100%;">
    <router-view />
  </div>
</template>
<script>
export default {
  created() {
    window.$wujie?.bus.$on("routeChange", (path) => {
      this.$router.push(path);
    });
  },
};
</script>

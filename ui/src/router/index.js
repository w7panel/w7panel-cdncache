import { createRouter, createWebHashHistory } from 'vue-router';

const routes = [
  {
    path: '/cache',
    name: 'cache',
    component: () => import('../views/cache/list.vue'),
  },
  {
    path: '/cache/storage',
    name: 'global-storage-setting',
    component: () => import('../views/settings/storage-config.vue'),
  },
  {
    path: '/cache/:host/:path*',
    name: 'cache-edit',
    component: () => import('../views/cache/index.vue'),
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/cache',
  },
];

const router = createRouter({
  history: createWebHashHistory(),
  routes,
});

export default router;

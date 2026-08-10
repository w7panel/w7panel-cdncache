import { createRouter, createWebHashHistory } from 'vue-router';

const routes = [
  {
    path: '/',
    name: 'public-home',
    component: () => import('../views/public/home.vue'),
  },
  {
    path: '/public',
    redirect: '/',
  },
  {
    path: '/cache',
    name: 'cache',
    component: () => import('../views/cache/list.vue'),
  },
  {
    path: '/cache/repository',
    name: 'cache-repository-setting',
    component: () => import('../views/settings/cache-repository.vue'),
  },
  {
    path: '/cache/page-setting',
    name: 'public-page-setting',
    component: () => import('../views/settings/page-setting.vue'),
  },
  {
    path: '/cache/:host/:path*',
    name: 'cache-edit',
    component: () => import('../views/cache/index.vue'),
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/',
  },
];

const router = createRouter({
  history: createWebHashHistory(),
  routes,
});

router.beforeEach((to) => {
  // 管理端作为无界微应用加载时仍保持原有默认入口；独立访问根路径才展示公开首页。
  if (to.name === 'public-home' && window.__POWERED_BY_WUJIE__) {
    return { name: 'cache' };
  }
  return true;
});

export default router;

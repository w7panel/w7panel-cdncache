import myAxios from "../utils/index";

const quietConfig = {
    dontalert: true,
};

const GLOBAL_GROUP = "global";

export const responseData = (response, fallback = {}) =>
    response?.data?.data ?? fallback;

const getGlobalSetting = (quiet = false) =>
    myAxios.post(
        "/api/setting/get",
        { group: GLOBAL_GROUP },
        quiet ? quietConfig : undefined,
    );

const getGlobalExtra = async (key, quiet = false) => {
    const response = await getGlobalSetting(quiet);
    const setting = responseData(response);
    return {
        data: {
            data: setting?.extra?.[key] || {},
        },
    };
};

const saveGlobalExtra = async (key, data) => {
    const response = await getGlobalSetting(true);
    const setting = responseData(response);

    return myAxios.post("/api/setting/set", {
        group: GLOBAL_GROUP,
        storage_source: setting.storage_source || {},
        minio: setting.minio || {},
        path_cache_rules: setting.path_cache_rules || [],
        path_key_cache_rules: setting.path_key_cache_rules || [],
        extra: {
            ...(setting.extra || {}),
            [key]: data,
        },
    });
};

export const getPublicSiteList = (quiet = false) =>
    myAxios.post("/api/setting/common-list", {}, quiet ? quietConfig : undefined);

export const getGlobalCacheRepository = (quiet = false) =>
    getGlobalExtra("cache_repository", quiet);

export const saveGlobalCacheRepository = (data) =>
    saveGlobalExtra("cache_repository", data);

export const getPageSetting = (quiet = false) =>
    getGlobalExtra("page_setting", quiet);

export const savePageSetting = (data) =>
    saveGlobalExtra("page_setting", data);

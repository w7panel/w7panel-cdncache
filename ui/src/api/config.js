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

export const getGlobalStorageConfig = async (quiet = false) => {
    const response = await getGlobalSetting(quiet);
    const setting = responseData(response);
    return {
        data: {
            data: setting?.minio || {},
        },
    };
};

export const saveGlobalStorageConfig = async (data) => {
    const response = await getGlobalSetting(true);
    const setting = responseData(response);
    const extra = { ...(setting.extra || {}) };
    delete extra.cache_repository;
    delete extra.page_setting;

    return myAxios.post("/api/setting/set", {
        group: GLOBAL_GROUP,
        storage_source: setting.storage_source || {},
        minio: data,
        path_cache_rules: setting.path_cache_rules || [],
        path_key_cache_rules: setting.path_key_cache_rules || [],
        extra,
    });
};

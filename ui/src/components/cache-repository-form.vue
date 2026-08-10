<template>
    <div class="mirror-repository-form">
        <a-form :model="modelValue" validate-trigger="blur" auto-label-width>
            <a-form-item label="镜像仓库地址">
                <el-input
                    :model-value="repositoryAddress"
                    :disabled="repositoryDisabled || disabled"
                    placeholder="请输入镜像仓库地址"
                    @update:model-value="updateRepositoryAddress"
                >
                    <template #prepend>
                        <el-select
                            :model-value="repositoryProtocol"
                            :disabled="repositoryDisabled || disabled"
                            placeholder="请选择"
                            class="protocol-select"
                            @change="updateRepositoryProtocol"
                        >
                            <el-option label="http://" value="http://" />
                            <el-option label="https://" value="https://" />
                        </el-select>
                    </template>
                </el-input>
                <template #extra>用于存放缓存镜像。</template>
            </a-form-item>

            <a-form-item label="存储目录">
                <a-input
                    :model-value="modelValue.storage_path"
                    :disabled="disabled"
                    placeholder="/"
                    @update:model-value="updateField('storage_path', $event)"
                    @blur="normalizeStoragePath"
                />
                <template #extra>目录必须以 / 开头，未填写时使用根目录 /。</template>
            </a-form-item>

            <a-form-item label="用户名">
                <a-input
                    :model-value="modelValue.username"
                    :disabled="inherited || disabled"
                    :spellcheck="false"
                    placeholder="请输入"
                    @update:model-value="updateField('username', $event)"
                />
            </a-form-item>

            <a-form-item label="密码">
                <a-input-password
                    :model-value="modelValue.password"
                    :disabled="inherited || disabled"
                    :spellcheck="false"
                    placeholder="请输入"
                    @update:model-value="updateField('password', $event)"
                />
            </a-form-item>
        </a-form>
    </div>
</template>

<script>
export default {
    name: "CacheRepositoryForm",
    props: {
        modelValue: {
            type: Object,
            default: () => ({
                repository_url: "",
                storage_path: "/",
                username: "",
                password: "",
            }),
        },
        repositoryDisabled: {
            type: Boolean,
            default: false,
        },
        disabled: {
            type: Boolean,
            default: false,
        },
        inherited: {
            type: Boolean,
            default: false,
        },
    },
    emits: ["update:modelValue"],
    computed: {
        repositoryProtocol() {
            return this.modelValue.repository_url?.match?.(/^(https?:\/\/)/)?.[0] || "http://";
        },
        repositoryAddress() {
            return this.modelValue.repository_url?.replace?.(/^https?:\/\//, "") || "";
        },
    },
    methods: {
        updateField(key, value) {
            this.$emit("update:modelValue", {
                ...this.modelValue,
                [key]: value,
            });
        },
        updateRepositoryProtocol(protocol) {
            this.updateField("repository_url", protocol + this.repositoryAddress);
        },
        updateRepositoryAddress(address) {
            const value = address.trim();
            this.updateField(
                "repository_url",
                /^(https?:\/\/)/.test(value) ? value : this.repositoryProtocol + value,
            );
        },
        normalizeStoragePath() {
            const value = this.modelValue.storage_path?.trim() || "/";
            this.updateField("storage_path", value.startsWith("/") ? value : `/${value}`);
        },
    },
};
</script>

<style scoped>
.mirror-repository-form {
    max-width: 760px;
}

.protocol-select {
    width: 100px;
}
</style>

import { cloneElement, isValidElement, useId, useState } from "react"
import { useParams, useNavigate } from "react-router-dom"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { adminApi, dishesApi, uploadApi, getUploadErrorMessage } from "@/api"
import type { Dish, DishInput } from "@/types"
import { ArrowLeft, Camera, Check, FilePenLine, LoaderCircle } from "lucide-react"
import PageHeader from "@/components/PageHeader"
import RequestState from "@/components/RequestState"
import AnimatedBottomSheet from "@/components/AnimatedBottomSheet"
import { errorMessage } from "@/api/client"
import { useAuthStore } from "@/store/useAuthStore"
import { useRecipeDraft } from "@/lib/use-recipe-draft"
import { formatIngredients, parseIngredients, formatSteps, parseSteps } from "@/lib/recipe-text"
import { asArray } from "@/lib/utils"
import toast from "react-hot-toast"

const DEFAULT_CATEGORIES = ["川菜", "湘菜", "贵州菜", "云南菜", "粤菜"]
const DEFAULT_TASTES = ["辣", "麻辣", "香辣", "酸辣", "鲜辣", "酸", "甜", "鲜", "清淡", "咸鲜", "蒜香", "葱香", "酱香", "豉香"]
const mealTypes = [
  { key: "lunch", label: "午餐" },
  { key: "dinner", label: "晚餐" },
  { key: "all", label: "通用" },
]
const difficulties = ["easy", "medium", "hard"]
const diffLabels = { easy: "简单", medium: "中等", hard: "困难" }

function parseList(raw: unknown, fallback: string[]): string[] {
  const arr = asArray<string>(raw).filter((x) => typeof x === "string")
  return arr.length > 0 ? arr : fallback
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mb-5">
      <div className="text-xs font-bold text-primary uppercase tracking-wider mb-2.5">{title}</div>
      <div className="bg-card rounded-2xl p-4 shadow-sm border border-border space-y-4">
        {children}
      </div>
    </div>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  const id = useId()
  const field = isValidElement<{ id?: string; "aria-describedby"?: string }>(children) && (children.type === "input" || children.type === "textarea")
    ? cloneElement(children, { id, "aria-describedby": hint ? `${id}-hint` : undefined }) : children
  return <div role="group" aria-label={label}>
    <label htmlFor={field !== children ? id : undefined} className="mb-2 block text-sm font-semibold text-text">{label}</label>
    {hint && <p id={`${id}-hint`} className="mb-2 text-xs leading-relaxed text-text2">{hint}</p>}
    {field}
  </div>
}

const inputCls = "w-full py-2.5 px-3.5 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none transition-all focus:border-primary focus:shadow-[0_0_0_3px_rgba(232,115,74,.1)]"
const textareaCls = "w-full py-2.5 px-3.5 rounded-[10px] border-[1.5px] border-border bg-bg text-sm outline-none min-h-[80px] resize-y leading-relaxed transition-all focus:border-primary"

export default function AdminDishEdit({ mode = "admin" }: { mode?: "admin" | "user" }) {
  const { id } = useParams()
  const navigate = useNavigate()
  const userId = useAuthStore((state) => state.user?.id || 0)
  const dishId = id ? Number(id) : 0
  const isNew = !id || id === "new"
  const validId = isNew || Number.isSafeInteger(dishId) && dishId > 0
  const query = useQuery({ queryKey: ["dish", dishId], queryFn: () => dishesApi.get(dishId), enabled: !isNew && validId })
  const denied = !isNew && query.isSuccess && !query.data.access?.can_edit
  if (!validId || denied || !isNew && (query.isPending || query.isError)) return <>
    <PageHeader title="编辑菜谱" onBack={() => navigate(mode === "admin" ? "/admin/dishes" : "/dishes?scope=mine")} />
    <RequestState error={!validId ? new Error("菜谱地址无效，请返回菜谱列表") : denied ? new Error("你没有这道菜谱的编辑权限，可以返回查看或创建自己的私房菜。") : query.error} onRetry={validId && !denied ? () => { void query.refetch() } : undefined} loading={query.isFetching} />
  </>
  return <DishEditForm key={`${userId}:${mode}:${dishId}`} userId={userId} mode={mode} dishId={dishId} isNew={isNew} dish={query.data} />
}

function DishEditForm({ userId, mode, dishId, isNew, dish }: { userId: number; mode: "admin" | "user"; dishId: number; isNew: boolean; dish?: Dish }) {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data: settings } = useQuery({
    queryKey: ["admin-site-settings"],
    queryFn: () => adminApi.settings(),
    enabled: mode === "admin",
  })
  const categories = parseList(settings?.categories, DEFAULT_CATEGORIES)
  const tastes = parseList(settings?.tastes, DEFAULT_TASTES)

  const [name, setName] = useState(dish?.name || "")
  const [category, setCategory] = useState(dish?.category || "川菜")
  const [mealType, setMealType] = useState(dish?.meal_type || "all")
  const [difficulty, setDifficulty] = useState(dish?.difficulty || "easy")
  const [tasteList, setTasteList] = useState<string[]>((dish?.taste || "").split(",").map((item) => item.trim()).filter(Boolean))
  const [cookTime, setCookTime] = useState(dish?.cook_time ?? 15)
  const [sourceDish] = useState(dish)
  const [ingredientsText, setIngredientsText] = useState(formatIngredients(asArray(dish?.ingredients)))
  const [seasoningsText, setSeasoningsText] = useState(formatIngredients(asArray(dish?.seasonings)))
  const [stepsText, setStepsText] = useState(formatSteps(asArray(dish?.steps)))
  const [remark, setRemark] = useState(dish?.remark || "")
  const [imageUrl, setImageUrl] = useState(dish?.image_url || "")
  const [images, setImages] = useState<string[]>(asArray<string>(dish?.images))
  const [videoUrl, setVideoUrl] = useState(dish?.video_url || "")
  const [tags, setTags] = useState<string[]>(asArray<string>(dish?.tags))
  const [tagInput, setTagInput] = useState("")
  const [sortOrder, setSortOrder] = useState(dish?.sort_order || 0)
  const [addingCategory, setAddingCategory] = useState(false)
  const [addingTaste, setAddingTaste] = useState(false)
  const [newCategory, setNewCategory] = useState("")
  const [newTaste, setNewTaste] = useState("")
  const [manageCategory, setManageCategory] = useState(false)
  const [manageTaste, setManageTaste] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<{ type: "category" | "taste"; value: string } | null>(null)

  const [uploading, setUploading] = useState(false)
  const draft = useRecipeDraft(userId, dishId, mode, { name, category, mealType, difficulty, tasteList, cookTime, ingredientsText, seasoningsText, stepsText, remark, imageUrl, images, videoUrl, tags, sortOrder }, (value) => {
    setName(value.name); setCategory(value.category); setMealType(value.mealType); setDifficulty(value.difficulty)
    setTasteList(value.tasteList); setCookTime(value.cookTime); setIngredientsText(value.ingredientsText)
    setSeasoningsText(value.seasoningsText); setStepsText(value.stepsText); setRemark(value.remark)
    setImageUrl(value.imageUrl); setImages(value.images); setVideoUrl(value.videoUrl); setTags(value.tags); setSortOrder(value.sortOrder)
  })
  const back = () => navigate(mode === "admin" ? "/admin/dishes" : isNew ? "/dishes?scope=mine" : `/dishes/${dishId}`)

  const saveMut = useMutation({
    mutationFn: () => {
      const data: DishInput = {
        name: name.trim(),
        category,
        meal_type: mealType,
        difficulty,
        taste: tasteList.join(","),
        cook_time: cookTime,
        ingredients: JSON.stringify(parseIngredients(ingredientsText, asArray(sourceDish?.ingredients))),
        seasonings: JSON.stringify(parseIngredients(seasoningsText, asArray(sourceDish?.seasonings))),
        steps: JSON.stringify(parseSteps(stepsText, asArray(sourceDish?.steps))),
        remark,
        image_url: imageUrl,
        images: JSON.stringify(images),
        video_url: videoUrl.trim(),
        tags: JSON.stringify(tags),
        sort_order: sortOrder,
      }
      if (isNew && mode === "admin") data.public = true
      return isNew ? dishesApi.create(data) : dishesApi.update(dishId, data)
    },
    onSuccess: () => {
      draft.clear()
      qc.invalidateQueries({ queryKey: ["dish", dishId] })
      qc.invalidateQueries({ queryKey: ["category-counts"] })
      qc.invalidateQueries({ queryKey: ["favorites"] })
      qc.invalidateQueries({ queryKey: ["family-dishes"] })
      qc.invalidateQueries({ queryKey: ["admin", "dishes"] })
      qc.invalidateQueries({ queryKey: ["dishes"] })
      toast.success("已保存")
      back()
    },
    onError: (error) => toast.error(errorMessage(error, "保存失败，草稿已保留")),
  })

  const addCategoryMut = useMutation({
    mutationFn: (val: string) => adminApi.updateSettings({ categories: JSON.stringify([...categories, val]) }),
    onSuccess: (_d, val) => {
      qc.invalidateQueries({ queryKey: ["admin-site-settings"] })
      setCategory(val)
      setNewCategory("")
      setAddingCategory(false)
      toast.success("已添加分类")
    },
    onError: () => toast.error("添加失败，请先登录后台"),
  })

  const addTasteMut = useMutation({
    mutationFn: (val: string) => adminApi.updateSettings({ tastes: JSON.stringify([...tastes, val]) }),
    onSuccess: (_d, val) => {
      qc.invalidateQueries({ queryKey: ["admin-site-settings"] })
      setTasteList((prev) => prev.includes(val) ? prev : [...prev, val])
      setNewTaste("")
      setAddingTaste(false)
      toast.success("已添加口味")
    },
    onError: () => toast.error("添加失败，请先登录后台"),
  })

  const delCategoryMut = useMutation({
    mutationFn: (val: string) => adminApi.updateSettings({ categories: JSON.stringify(categories.filter((c) => c !== val)) }),
    onSuccess: (_d, val) => {
      qc.invalidateQueries({ queryKey: ["admin-site-settings"] })
      if (category === val) setCategory("")
      setPendingDelete(null)
      toast.success("已删除分类")
    },
    onError: () => toast.error("删除失败，请先登录后台"),
  })

  const delTasteMut = useMutation({
    mutationFn: (val: string) => adminApi.updateSettings({ tastes: JSON.stringify(tastes.filter((t) => t !== val)) }),
    onSuccess: (_d, val) => {
      qc.invalidateQueries({ queryKey: ["admin-site-settings"] })
      setTasteList((prev) => prev.filter((x) => x !== val))
      setPendingDelete(null)
      toast.success("已删除口味")
    },
    onError: () => toast.error("删除失败，请先登录后台"),
  })

  function confirmDelete() {
    if (!pendingDelete) return
    if (pendingDelete.type === "category") delCategoryMut.mutate(pendingDelete.value)
    else delTasteMut.mutate(pendingDelete.value)
  }

  function toggleTaste(t: string) {
    setTasteList((prev) => prev.includes(t) ? prev.filter((x) => x !== t) : [...prev, t])
  }

  async function handleImageUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ""
    if (!file || uploading) return
    setUploading(true)
    try {
      const res = await uploadApi.image(file)
      setImageUrl(res.data.data.url)
      toast.success("主图已上传")
    } catch (err) { toast.error(getUploadErrorMessage(err)) }
    finally { setUploading(false) }
  }

  async function handleExtraImageUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files || [])
    e.target.value = ""
    if (!files.length || uploading) return
    if (files.length + images.length > 18) { toast.error("最多保存 18 张图片"); return }
    setUploading(true)
    let uploaded = 0
    try {
      for (const file of files) {
        const res = await uploadApi.image(file)
        setImages((prev) => [...prev, res.data.data.url])
        uploaded++
      }
      toast.success(`已上传 ${uploaded} 张图片`)
    } catch (err) { toast.error(`${uploaded ? `已上传 ${uploaded} 张，其余` : ""}${getUploadErrorMessage(err)}`) }
    finally { setUploading(false) }
  }

  function addTag() {
    const t = tagInput.trim()
    if (t && !tags.includes(t)) {
      setTags([...tags, t])
    }
    setTagInput("")
  }

  const pillClass = (active: boolean) =>
    `inline-flex min-h-11 items-center rounded-full text-sm border-[1.5px] transition-all ${active ? "bg-primary text-white border-primary" : "border-border text-text2"}`

  const disabled = !name.trim() || saveMut.isPending || uploading || Boolean(draft.recovery)
  return (
    <>
      <PageHeader title={isNew ? (mode === "admin" ? "添加公共菜谱" : "新建私房菜") : "编辑菜谱"}
        subtitle={dish?.family_id ? "家庭成员可见" : mode === "admin" || dish?.owner_id === 0 ? "公共菜谱" : "仅自己可见"} onBack={back} />
      <div className="mx-auto max-w-[640px] px-5 py-5 pb-[calc(100px+env(safe-area-inset-bottom))]">
        {draft.recovery && <div className="draft-notice" role="status">
          <div className="flex items-center gap-2 font-semibold"><FilePenLine size={18} />有一份未完成的草稿</div>
          <p>保存于 {new Date(draft.recovery.savedAt).toLocaleString("zh-CN")}。恢复后可继续编辑，保存才会更新菜谱。</p>
          <div className="draft-notice__actions"><button type="button" onClick={draft.resume} className="btn-secondary">继续编辑草稿</button><button type="button" onClick={draft.discard} className="btn-secondary">丢弃旧草稿</button></div>
        </div>}
        <div className="draft-status mb-4" aria-live="polite"><FilePenLine size={14} />{draft.status}</div>
        <fieldset disabled={Boolean(draft.recovery) || saveMut.isPending} className="min-w-0">
      <Section title="基本信息">
        <Field label="菜品名称 *" hint="必填">
          <input type="text" maxLength={100} value={name} onChange={(e) => setName(e.target.value)} placeholder="如：番茄炒蛋" className={inputCls} />
        </Field>

        <Field label="分类">
          <div className="flex items-center justify-between mb-1.5">
            <span />
            {mode === "admin" && <button onClick={() => setManageCategory((v) => !v)} className={`text-[12px] px-2 py-0.5 rounded-full transition-all ${manageCategory ? "bg-primary text-white" : "text-text3 hover:text-primary"}`}>{manageCategory ? "完成" : "管理"}</button>}
          </div>
          <div className="flex flex-wrap gap-2 items-center">
            {categories.map((c) => (
              <span key={c} className={pillClass(category === c && !manageCategory)}>
                <button onClick={() => !manageCategory && setCategory(c)} className="pl-3.5 pr-2 py-1.5 active:scale-95">{c}</button>
                {manageCategory && (
                  <button onClick={() => setPendingDelete({ type: "category", value: c })} className="pr-2.5 pl-0.5 py-1.5 text-text3 hover:text-red-500">✕</button>
                )}
              </span>
            ))}
            {mode === "admin" && addingCategory ? (
              <div className="flex items-center gap-1">
                <input autoFocus value={newCategory} onChange={(e) => setNewCategory(e.target.value)}
                  onKeyDown={(e) => { if (e.key === "Enter" && newCategory.trim()) addCategoryMut.mutate(newCategory.trim()); if (e.key === "Escape") setAddingCategory(false) }}
                  placeholder="新分类" className="w-24 px-2.5 py-1.5 rounded-full text-xs border-[1.5px] border-primary bg-bg outline-none" />
                <button onClick={() => newCategory.trim() && addCategoryMut.mutate(newCategory.trim())} className="px-2.5 py-1.5 rounded-full text-xs bg-primary text-white">✓</button>
                <button onClick={() => { setAddingCategory(false); setNewCategory("") }} className="px-2.5 py-1.5 rounded-full text-xs border border-border text-text2">✕</button>
              </div>
            ) : mode === "admin" ? (
              <button onClick={() => setAddingCategory(true)} className="px-3.5 py-1.5 rounded-full text-xs border-[1.5px] border-dashed border-border2 text-text2">+ 自定义</button>
            ) : null}
          </div>
        </Field>

        <div className="grid grid-cols-2 gap-4">
          <Field label="适合餐次">
            <div className="flex flex-wrap gap-2">
              {mealTypes.map((m) => (
                <button key={m.key} onClick={() => setMealType(m.key)} className={`px-3.5 py-1.5 rounded-full text-xs border-[1.5px] transition-all active:scale-95 ${mealType === m.key ? "bg-primary text-white border-primary" : "border-border text-text2"}`}>{m.label}</button>
              ))}
            </div>
          </Field>
          <Field label="难度">
            <div className="flex flex-wrap gap-2">
              {difficulties.map((d) => (
                <button key={d} onClick={() => setDifficulty(d)} className={`px-3.5 py-1.5 rounded-full text-xs border-[1.5px] transition-all active:scale-95 ${difficulty === d ? "bg-primary text-white border-primary" : "border-border text-text2"}`}>{diffLabels[d as keyof typeof diffLabels]}</button>
              ))}
            </div>
          </Field>
        </div>

        <Field label="口味（可多选）">
          <div className="flex items-center justify-between mb-1.5">
            <span />
            {mode === "admin" && <button onClick={() => setManageTaste((v) => !v)} className={`text-[12px] px-2 py-0.5 rounded-full transition-all ${manageTaste ? "bg-primary text-white" : "text-text3 hover:text-primary"}`}>{manageTaste ? "完成" : "管理"}</button>}
          </div>
          <div className="flex flex-wrap gap-2 items-center">
            {tastes.map((t) => (
              <span key={t} className={pillClass(tasteList.includes(t) && !manageTaste)}>
                <button onClick={() => !manageTaste && toggleTaste(t)} className="pl-3.5 pr-2 py-1.5 active:scale-95">{t}</button>
                {manageTaste && (
                  <button onClick={() => setPendingDelete({ type: "taste", value: t })} className="pr-2.5 pl-0.5 py-1.5 text-text3 hover:text-red-500">✕</button>
                )}
              </span>
            ))}
            {mode === "admin" && addingTaste ? (
              <div className="flex items-center gap-1">
                <input autoFocus value={newTaste} onChange={(e) => setNewTaste(e.target.value)}
                  onKeyDown={(e) => { if (e.key === "Enter" && newTaste.trim()) addTasteMut.mutate(newTaste.trim()); if (e.key === "Escape") setAddingTaste(false) }}
                  placeholder="新口味" className="w-24 px-2.5 py-1.5 rounded-full text-xs border-[1.5px] border-primary bg-bg outline-none" />
                <button onClick={() => newTaste.trim() && addTasteMut.mutate(newTaste.trim())} className="px-2.5 py-1.5 rounded-full text-xs bg-primary text-white">✓</button>
                <button onClick={() => { setAddingTaste(false); setNewTaste("") }} className="px-2.5 py-1.5 rounded-full text-xs border border-border text-text2">✕</button>
              </div>
            ) : mode === "admin" ? (
              <button onClick={() => setAddingTaste(true)} className="px-3.5 py-1.5 rounded-full text-xs border-[1.5px] border-dashed border-border2 text-text2">+ 自定义</button>
            ) : null}
          </div>
        </Field>

        <div className="grid grid-cols-2 gap-4">
          <Field label="烹饪时间（分钟）">
            <input type="number" value={cookTime} onChange={(e) => setCookTime(Number(e.target.value))} min={0} max={600} className={inputCls} />
          </Field>
          <Field label="排序权重" hint="数值越大越靠前">
            <input type="number" value={sortOrder} onChange={(e) => setSortOrder(Number(e.target.value))} className={inputCls} />
          </Field>
        </div>
      </Section>

      <Section title="图片与视频">
        <Field label="主图" hint="点击上传或替换菜品主图（支持 jpg/png/webp，最大5MB）">
          <label className="block border-2 border-dashed border-border2 rounded-2xl overflow-hidden text-center cursor-pointer transition-all hover:border-primary hover:bg-primary-light">
            <input type="file" disabled={uploading} accept="image/jpeg,image/png,image/webp" onChange={handleImageUpload} className="hidden" />
            {imageUrl ? (
              <img src={imageUrl} alt="预览" className="w-full h-48 object-cover" />
            ) : (
              <div className="p-6">
                <Camera size={32} className="mx-auto mb-2 text-text2" />
                <div className="text-[13px] text-text2">点击上传主图</div>
              </div>
            )}
          </label>
        </Field>

        <Field label="额外图片" hint="可上传多张菜品图片">
          <label className="block border-2 border-dashed border-border2 rounded-xl overflow-hidden text-center cursor-pointer py-3 px-4 transition-all hover:border-primary hover:bg-primary-light">
            <input type="file" disabled={uploading} accept="image/jpeg,image/png,image/webp" multiple onChange={handleExtraImageUpload} className="hidden" />
            <span className="text-xs text-text2">+ 点击上传更多图片（可多选）</span>
          </label>
          {images.length > 0 && (
            <div className="flex gap-2 mt-3 overflow-x-auto scrollbar-hide">
              {images.map((img, i) => (
                <div key={i} className="relative flex-shrink-0 w-20 h-20 rounded-xl overflow-hidden group">
                  <img src={img} alt="" className="w-full h-full object-cover" />
                  <button
                    aria-label={`删除第 ${i + 1} 张图片`} onClick={() => setImages(images.filter((_, j) => j !== i))}
                    className="absolute top-0 right-0 w-10 h-10 rounded-full bg-black/60 text-white text-xs flex items-center justify-center opacity-100 transition-opacity"
                  >✕</button>
                </div>
              ))}
            </div>
          )}
        </Field>

        <Field label="教程视频链接" hint="仅支持抖音 / 哔哩哔哩，保存后可从详情页直接打开播放">
          <input type="text" maxLength={2048} value={videoUrl} onChange={(e) => setVideoUrl(e.target.value)} placeholder="粘贴视频链接" className={inputCls} />
        </Field>
      </Section>

      <Section title="食材与步骤">
        <Field label="配料清单" hint="每行：名称 数量，最后一段为数量">
          <textarea value={ingredientsText} onChange={(e) => setIngredientsText(e.target.value)} rows={4} placeholder={"番茄 2个\n鸡蛋 3个"} className={textareaCls} />
        </Field>

        <Field label="调料清单" hint="每行：名称 数量">
          <textarea value={seasoningsText} onChange={(e) => setSeasoningsText(e.target.value)} rows={2} placeholder={"盐 1茶匙\n白糖 1茶匙"} className={textareaCls} />
        </Field>

        <Field label="制作步骤" hint="每行一步，可选(分钟数)标注时间">
          <textarea value={stepsText} onChange={(e) => setStepsText(e.target.value)} rows={4} placeholder={"1. 番茄洗净切块 (3分钟)\n2. 倒入蛋液翻炒"} className={textareaCls} />
        </Field>
      </Section>

      <Section title="标签与备注">
        <Field label="标签" hint="添加自定义标签方便筛选">
          <div className="flex flex-wrap gap-2 mb-2">
            {tags.map((t) => (
              <span key={t} className="inline-flex items-center gap-1 px-3 py-1 rounded-full text-xs bg-primary-light text-primary font-semibold">
                {t}
                <button onClick={() => setTags(tags.filter((x) => x !== t))} className="hover:text-red-500">✕</button>
              </span>
            ))}
          </div>
          <div className="flex gap-2">
            <input
              type="text" value={tagInput} onChange={(e) => setTagInput(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter" && !e.nativeEvent.isComposing) { e.preventDefault(); addTag() } }}
              placeholder="输入标签后回车添加" className={`flex-1 ${inputCls}`}
            />
            <button onClick={addTag} className="px-4 rounded-full text-xs font-semibold bg-primary text-white">添加</button>
          </div>
        </Field>

        <Field label="备注" hint="烹饪小贴士、注意事项等">
          <textarea maxLength={500} value={remark} onChange={(e) => setRemark(e.target.value)} rows={2} placeholder="一些小贴士..." className={`${textareaCls} min-h-[60px]`} />
        </Field>
      </Section>

        </fieldset>
      </div>
      <div className="fixed inset-x-0 bottom-0 z-[110] border-t border-border bg-card px-5 py-3 pb-[calc(12px+env(safe-area-inset-bottom))]">
        <div className="mx-auto flex max-w-[600px] gap-3">
          <button type="button" onClick={back} disabled={saveMut.isPending} className="btn-secondary"><ArrowLeft size={16} />返回</button>
          <button type="button" onClick={() => saveMut.mutate()} disabled={disabled} className="flex min-h-12 flex-1 items-center justify-center gap-2 rounded-2xl bg-primary px-5 text-sm font-semibold text-white">
            {saveMut.isPending || uploading ? <LoaderCircle size={18} className="animate-spin" /> : <Check size={18} />}
            {uploading ? "图片上传中…" : saveMut.isPending ? "保存中…" : isNew ? "创建菜谱" : "保存修改"}
          </button>
        </div>
      </div>

      {pendingDelete && (
        <AnimatedBottomSheet label="删除分类或口味" onClose={() => setPendingDelete(null)}>
          <div className="p-6 pb-8">
            <div className="text-base font-bold mb-1.5">删除{pendingDelete.type === "category" ? "分类" : "口味"}</div>
            <div className="text-sm text-text2 mb-5">确定删除「{pendingDelete.value}」吗？此操作不可撤销。</div>
            <div className="flex gap-2.5">
              <button onClick={() => setPendingDelete(null)} className="flex-1 py-2.5 px-5 rounded-full text-sm font-semibold border-[1.5px] border-border2">取消</button>
              <button onClick={confirmDelete} disabled={delCategoryMut.isPending || delTasteMut.isPending} className="flex-1 py-2.5 px-5 rounded-full text-sm font-semibold bg-red-500 text-white disabled:opacity-50">删除</button>
            </div>
          </div>
        </AnimatedBottomSheet>
      )}
    </>
  )
}

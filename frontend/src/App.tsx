import { lazy } from 'react'
import { Routes, Route } from 'react-router'
import Home from './pages/Home'
import Layout from './components/layout/Layout'

const Login = lazy(() => import('./pages/Login'))
const Upload = lazy(() => import('./pages/Upload'))
const Callback = lazy(() => import('./pages/Callback'))
const Gallery = lazy(() => import('./pages/Gallery'))
const Tags = lazy(() => import('./pages/Tags'))
const Artists = lazy(() => import('./pages/Artists'))
const Albums = lazy(() => import('./pages/Albums'))
const AlbumPage = lazy(() => import('./pages/AlbumPage'))
const ImagePage = lazy(() => import('./pages/ImagePage'))
const Review = lazy(() => import('./pages/Review'))
const Users = lazy(() => import('./pages/Users'))
const ApiKeys = lazy(() => import('./pages/ApiKeys'))
const Reports = lazy(() => import('./pages/Reports'))
const MyReports = lazy(() => import('./pages/MyReports'))
const AdminStats = lazy(() => import('./pages/AdminStats'))
const StorageDiff = lazy(() => import('./pages/StorageDiff'))
const Contact = lazy(() => import('./pages/Contact'))
const Imprint = lazy(() => import('./pages/Imprint'))
const Privacy = lazy(() => import('./pages/Privacy'))
const SiteBannerSettings = lazy(() => import('./pages/SiteBannerSettings'))
const NotFound = lazy(() => import('./pages/NotFound'))
const ArtistPage = lazy(() => import('./pages/ArtistPage'))
const TagPage = lazy(() => import('./pages/TagPage'))

function App() {
    return (
        <Routes>
            <Route path="/" element={<Layout />}>
                <Route index element={<Home />} />
                <Route path="gallery" element={<Gallery />} />
                <Route path="albums" element={<Albums />} />
                <Route path="albums/:id" element={<AlbumPage />} />
                <Route path="users/:userId/albums" element={<Albums />} />
                <Route path="users/:userId/albums/:id" element={<AlbumPage />} />
                <Route path="images/:id" element={<ImagePage />} />
                <Route path="tags" element={<Tags />} />
                <Route path="tags/:slug" element={<TagPage />} />
                <Route path="artists" element={<Artists />} />
                <Route path="artists/:id" element={<ArtistPage />} />
                <Route path="login" element={<Login />} />
                <Route path="upload" element={<Upload />} />
                <Route path="review" element={<Review />} />
                <Route path="my-reports" element={<MyReports />} />
                <Route path="reports" element={<Reports />} />
                <Route path="users" element={<Users />} />
                <Route path="api-keys" element={<ApiKeys />} />
                <Route path="stats" element={<AdminStats />} />
                <Route path="storage-diff" element={<StorageDiff />} />
                <Route path="site-banner" element={<SiteBannerSettings />} />
                <Route path="contact" element={<Contact />} />
                <Route path="imprint" element={<Imprint />} />
                <Route path="privacy" element={<Privacy />} />
                <Route path="auth/callback" element={<Callback />} />
                <Route path="*" element={<NotFound />} />
            </Route>
        </Routes>
    )
}

export default App

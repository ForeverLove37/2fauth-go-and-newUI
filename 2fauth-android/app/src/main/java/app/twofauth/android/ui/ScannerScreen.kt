package app.twofauth.android.ui

import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.CompositingStrategy
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import app.twofauth.android.R
import app.twofauth.android.data.QrDecoder
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

/** A live QR scanner. Frames are decoded on the phone and nothing is recorded. */
@Composable
fun ScannerScreen(onResult: (String) -> Unit, onBack: () -> Unit) {
    val context = LocalContext.current
    val lifecycle = LocalLifecycleOwner.current
    var granted by remember {
        mutableStateOf(ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED)
    }
    val ask = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted = it }
    LaunchedEffect(Unit) { if (!granted) ask.launch(Manifest.permission.CAMERA) }
    BackHandler(onBack = onBack)

    Box(Modifier.fillMaxSize().background(Color.Black)) {
        if (granted) {
            val done = remember { AtomicBoolean(false) }
            val executor = remember { Executors.newSingleThreadExecutor() }
            DisposableEffect(Unit) { onDispose { executor.shutdown() } }
            AndroidView(
                modifier = Modifier.fillMaxSize(),
                factory = { viewContext ->
                    val view = PreviewView(viewContext).apply { scaleType = PreviewView.ScaleType.FILL_CENTER }
                    val future = ProcessCameraProvider.getInstance(viewContext)
                    future.addListener({
                        val provider = future.get()
                        val preview = Preview.Builder().build().also { it.setSurfaceProvider(view.surfaceProvider) }
                        val analysis = ImageAnalysis.Builder()
                            .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                            .build()
                        analysis.setAnalyzer(executor) { image ->
                            image.use {
                                if (done.get()) return@use
                                val plane = it.planes[0]
                                val bytes = ByteArray(plane.buffer.remaining()).also { array -> plane.buffer.get(array) }
                                val text = QrDecoder.fromFrame(bytes, it.width, it.height, plane.rowStride)
                                if (text != null && done.compareAndSet(false, true)) {
                                    ContextCompat.getMainExecutor(viewContext).execute { onResult(text) }
                                }
                            }
                        }
                        runCatching {
                            provider.unbindAll()
                            provider.bindToLifecycle(lifecycle, CameraSelector.DEFAULT_BACK_CAMERA, preview, analysis)
                        }
                    }, ContextCompat.getMainExecutor(viewContext))
                    view
                },
                onRelease = { runCatching { ProcessCameraProvider.getInstance(context).get().unbindAll() } },
            )
            Viewfinder()
        } else {
            Column(Modifier.align(Alignment.Center).padding(32.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                Text(stringResource(R.string.camera_needed), color = Color.White, textAlign = TextAlign.Center)
                Button(onClick = { ask.launch(Manifest.permission.CAMERA) }, colors = primaryButton(), modifier = Modifier.padding(top = 16.dp)) {
                    Text(stringResource(R.string.allow_camera))
                }
            }
        }
        Row(Modifier.statusBarsPadding().padding(4.dp), verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, stringResource(R.string.back), tint = Color.White) }
            Text(stringResource(R.string.scan_qr), color = Color.White, fontWeight = FontWeight.SemiBold)
        }
        Text(
            stringResource(R.string.scan_hint),
            color = Color.White.copy(alpha = 0.85f),
            fontSize = 13.sp,
            textAlign = TextAlign.Center,
            modifier = Modifier.align(Alignment.BottomCenter).navigationBarsPadding().padding(32.dp),
        )
    }
}

/** A dimmed screen with a clear square, its corners in the accent, and a sweeping line. */
@Composable
private fun Viewfinder() {
    val sweep by rememberInfiniteTransition(label = "sweep").animateFloat(
        0.08f, 0.92f, infiniteRepeatable(tween(1600, easing = LinearEasing), RepeatMode.Reverse), label = "line",
    )
    val accent = Palette.accent
    Canvas(Modifier.fillMaxSize().graphicsLayer { compositingStrategy = CompositingStrategy.Offscreen }) {
        val side = size.minDimension * 0.68f
        val topLeft = Offset((size.width - side) / 2, (size.height - side) / 2.3f)
        val corner = CornerRadius(28.dp.toPx())
        drawRect(Color.Black.copy(alpha = 0.55f))
        drawRoundRect(Color.Transparent, topLeft, Size(side, side), corner, blendMode = BlendMode.Clear)
        drawRoundRect(accent, topLeft, Size(side, side), corner, style = Stroke(3.dp.toPx()))
        val y = topLeft.y + side * sweep
        drawLine(accent.copy(alpha = 0.8f), Offset(topLeft.x + 16.dp.toPx(), y), Offset(topLeft.x + side - 16.dp.toPx(), y), 2.dp.toPx())
    }
}

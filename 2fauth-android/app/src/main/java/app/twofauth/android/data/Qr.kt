package app.twofauth.android.data

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.LuminanceSource
import com.google.zxing.MultiFormatReader
import com.google.zxing.NotFoundException
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.RGBLuminanceSource
import com.google.zxing.common.GlobalHistogramBinarizer
import com.google.zxing.common.HybridBinarizer

/** QR codes read on the phone: a secret that should stay here is never sent to be decoded. */
object QrDecoder {
    private val hints = mapOf(
        DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE),
        DecodeHintType.TRY_HARDER to true,
    )

    private fun read(source: LuminanceSource): String? {
        val reader = MultiFormatReader().apply { setHints(hints) }
        for (binarizer in listOf(HybridBinarizer(source), GlobalHistogramBinarizer(source))) {
            try {
                return reader.decodeWithState(BinaryBitmap(binarizer)).text
            } catch (_: NotFoundException) {
                reader.reset()
            } catch (_: Exception) {
                reader.reset()
            }
        }
        return null
    }

    /** One camera frame: its luminance plane is all a QR code needs. */
    fun fromFrame(luminance: ByteArray, width: Int, height: Int, rowStride: Int): String? =
        read(PlanarYUVLuminanceSource(luminance, rowStride, height, 0, 0, width, height, false))

    fun fromBitmap(bitmap: Bitmap): String? {
        val pixels = IntArray(bitmap.width * bitmap.height)
        bitmap.getPixels(pixels, 0, bitmap.width, 0, 0, bitmap.width, bitmap.height)
        return read(RGBLuminanceSource(bitmap.width, bitmap.height, pixels))
    }

    /** A picture from the gallery, scaled down first: a screenshot is plenty at 1600 pixels. */
    fun fromUri(context: Context, uri: Uri): String? = runCatching {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        context.contentResolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, bounds) }
        var sample = 1
        while (maxOf(bounds.outWidth, bounds.outHeight) / (sample * 2) >= 1600) sample *= 2
        val bitmap = context.contentResolver.openInputStream(uri)?.use {
            BitmapFactory.decodeStream(it, null, BitmapFactory.Options().apply { inSampleSize = sample })
        } ?: return null
        fromBitmap(bitmap).also { bitmap.recycle() }
    }.getOrNull()
}
